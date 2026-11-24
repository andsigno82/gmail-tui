package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"text/tabwriter"
)

// topFrames e il numero di frame lenti elencati nel riepilogo.
const topFrames = 10

// median e la mediana di vals (µs); 0 se vuoto. Non modifica vals.
func median(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	s := sortedCopy(vals)
	if n := len(s); n%2 == 0 {
		return (s[n/2-1] + s[n/2]) / 2
	}
	return s[len(s)/2]
}

// percentile e il percentile p (nearest-rank) di vals; 0 se vuoto.
func percentile(vals []int64, p float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	s := sortedCopy(vals)
	i := int(math.Ceil(p/100*float64(len(s)))) - 1
	return s[min(max(i, 0), len(s)-1)]
}

func sortedCopy(vals []int64) []int64 {
	s := append([]int64(nil), vals...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

func maxOf(vals []int64) int64 {
	var m int64
	for _, v := range vals {
		m = max(m, v)
	}
	return m
}

func sumOf(vals []int64) int64 {
	var s int64
	for _, v := range vals {
		s += v
	}
	return s
}

// ms formatta microsecondi come millisecondi con un decimale.
func ms(us int64) string { return fmt.Sprintf("%.1f", float64(us)/1000) }

// kib formatta byte come KiB con un decimale.
func kib(b float64) string { return fmt.Sprintf("%.1f", b/1024) }

// secs formatta microsecondi come secondi.
func secs(us int64) string { return fmt.Sprintf("%.2f s", float64(us)/1e6) }

// group raccoglie le misure di un'operazione.
type group struct {
	dur, wait, upd, view, tot, bytes, viewBytes []int64
	errs                                        int
}

// stepGroup raccoglie le misure di un passo (operazione, passo e ruolo uguali).
type stepGroup struct {
	label, role string
	dur         []int64
}

// frame e un messaggio Bubble Tea con Update+View, per la classifica dei lenti.
type frame struct {
	label, screen string
	t, upd, view  int64
	viewBytes     int
}

// Summarize legge una traccia JSONL e stampa il riepilogo (Piano 16): per
// operazione conteggio, mediana, p95 e max; le attese del gate; i frame piu
// lenti; i byte medi per View; il rendering del corpo; l'avvio. Le righe non
// valide si contano e si saltano; un file senza eventi e un errore.
func Summarize(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var (
		meta                       Event
		n, bad                     int
		lastT                      int64
		store, msgs, body          = map[string]*group{}, map[string]*group{}, map[string]*group{}
		storeOrder, msgOrder, bOrd []string
		frames                     []frame
		phases                     []Event
		firstStore                 = map[string]Event{}
		viewBytesAll               []int64
		steps                      = map[string]*stepGroup{}
		stepOrder                  []string
		connect                    []Event // passi del Connect, per la cronologia dell'avvio
	)
	get := func(m map[string]*group, order *[]string, k string) *group {
		g, ok := m[k]
		if !ok {
			g = &group{}
			m[k] = g
			*order = append(*order, k)
		}
		return g
	}

	// Piano 16, Task 7: esiti della cache per tipo (Op) e durate dell'istantanea mostrata.
	type cacheStat struct {
		hit, miss, epoch int
		age              []int64
	}
	var cacheOrder []string
	cacheOps := map[string]*cacheStat{}

	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || e.K == "" {
			bad++
			continue
		}
		n++
		lastT = max(lastT, e.T)
		switch e.K {
		case "meta":
			meta = e
		case "store":
			label := e.Op
			if e.Role != "" {
				label += " [" + e.Role + "]"
			}
			g := get(store, &storeOrder, label)
			g.dur = append(g.dur, e.Dur)
			g.wait = append(g.wait, e.Wait)
			if e.Err {
				g.errs++
			}
			if _, ok := firstStore[e.Op]; !ok {
				firstStore[e.Op] = e
			}
		case "step":
			key := e.Op + "\x00" + e.Step + "\x00" + e.Role
			g, ok := steps[key]
			if !ok {
				g = &stepGroup{label: e.Op + " " + e.Step, role: e.Role}
				steps[key] = g
				stepOrder = append(stepOrder, key)
			}
			g.dur = append(g.dur, e.Dur)
			if e.Op == "Connect" {
				connect = append(connect, e)
			}
		case "msg":
			label := e.Op
			if e.Key != "" {
				label += " " + e.Key
			}
			g := get(msgs, &msgOrder, label)
			g.upd = append(g.upd, e.Upd)
			g.view = append(g.view, e.View)
			g.tot = append(g.tot, e.Upd+e.View)
			if e.View > 0 || e.ViewBytes > 0 {
				g.viewBytes = append(g.viewBytes, int64(e.ViewBytes))
				viewBytesAll = append(viewBytesAll, int64(e.ViewBytes))
			}
			frames = append(frames, frame{label: label, screen: e.Screen, t: e.T, upd: e.Upd, view: e.View, viewBytes: e.ViewBytes})
		case "cache":
			c, ok := cacheOps[e.Op]
			if !ok {
				c = &cacheStat{}
				cacheOps[e.Op] = c
				cacheOrder = append(cacheOrder, e.Op)
			}
			switch e.Mode {
			case ModeHit:
				c.hit++
			case ModeMiss:
				c.miss++
			case ModeEpoch:
				c.epoch++
			case ModeAge:
				c.age = append(c.age, e.Dur)
			}
		case "phase":
			phases = append(phases, e)
		case "body":
			label := e.Op
			if e.Mode != "" {
				label += " " + e.Mode
			}
			g := get(body, &bOrd, label)
			g.dur = append(g.dur, e.Dur)
			g.bytes = append(g.bytes, int64(e.Bytes))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("traccia vuota o non valida")
	}

	fmt.Fprintf(w, "gmail-tui trace: %s/%s, terminale %s, versione %s, avvio %s\n", meta.OS, meta.Arch, meta.Term, meta.Version, meta.Start)
	fmt.Fprintf(w, "eventi: %d, durata: %s", n, secs(lastT))
	if bad > 0 {
		fmt.Fprintf(w, ", righe non valide: %d", bad)
	}
	fmt.Fprintln(w)

	// Avvio: fasi esplicite piu il primo Connect e il primo ListConversations.
	var start []Event
	for _, op := range []string{"Connect", "ListConversations"} {
		if e, ok := firstStore[op]; ok {
			e.Op = "store " + op + " (fine)"
			start = append(start, e)
		}
	}
	for _, e := range connect {
		e.Op = "Connect " + e.Step + " (passo)"
		start = append(start, e)
	}
	start = append(start, phases...)
	if len(start) > 0 {
		sort.SliceStable(start, func(i, j int) bool { return start[i].T < start[j].T })
		fmt.Fprintln(w, "\nAvvio (tempo dall'apertura della traccia)")
		for _, e := range start {
			fmt.Fprintf(w, "  +%s  %s\n", secs(e.T), e.Op)
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(storeOrder) > 0 {
		fmt.Fprintln(w, "\nOperazioni MailStore (ms; attesa gate = coda sulla connessione IMAP, esecuzione = durata - attesa)")
		fmt.Fprintln(tw, "operazione\tn\tmediana\tp95\tmax\tattesa gate med\tattesa gate p95\tattesa gate max\terr")
		for _, k := range storeOrder {
			g := store[k]
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", k, len(g.dur),
				ms(median(g.dur)), ms(percentile(g.dur, 95)), ms(maxOf(g.dur)),
				ms(median(g.wait)), ms(percentile(g.wait, 95)), ms(maxOf(g.wait)), g.errs)
		}
		_ = tw.Flush()
	}

	if len(stepOrder) > 0 {
		fmt.Fprintln(w, "\nPassi IMAP (ms; un comando per passo, per ruolo della cartella)")
		fmt.Fprintln(tw, "operazione passo\truolo\tn\tmediana\tp95\tmax")
		for _, k := range stepOrder {
			g := steps[k]
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", g.label, g.role, len(g.dur),
				ms(median(g.dur)), ms(percentile(g.dur, 95)), ms(maxOf(g.dur)))
		}
		_ = tw.Flush()
	}

	if len(msgOrder) > 0 {
		fmt.Fprintln(w, "\nMessaggi Bubble Tea (ms)")
		fmt.Fprintln(tw, "messaggio\tn\tUpdate med\tUpdate p95\tView med\tView p95\ttotale max\tKiB per View")
		for _, k := range msgOrder {
			g := msgs[k]
			avg := 0.0
			if len(g.viewBytes) > 0 {
				avg = float64(sumOf(g.viewBytes)) / float64(len(g.viewBytes))
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n", k, len(g.tot),
				ms(median(g.upd)), ms(percentile(g.upd, 95)), ms(median(g.view)), ms(percentile(g.view, 95)),
				ms(maxOf(g.tot)), kib(avg))
		}
		_ = tw.Flush()

		sort.SliceStable(frames, func(i, j int) bool { return frames[i].upd+frames[i].view > frames[j].upd+frames[j].view })
		fmt.Fprintf(w, "\nFrame piu lenti (primi %d, Update + View)\n", topFrames)
		for i, f := range frames {
			if i == topFrames {
				break
			}
			scr := ""
			if f.screen != "" {
				scr = " [" + f.screen + "]"
			}
			fmt.Fprintf(w, "  %7s ms  +%s  %s%s  (Update %s + View %s, %s KiB)\n",
				ms(f.upd+f.view), secs(f.t), f.label, scr, ms(f.upd), ms(f.view), kib(float64(f.viewBytes)))
		}
		avg := 0.0
		if len(viewBytesAll) > 0 {
			avg = float64(sumOf(viewBytesAll)) / float64(len(viewBytesAll))
		}
		fmt.Fprintf(w, "\nByte medi per View: %s KiB (%d View)\n", kib(avg), len(viewBytesAll))
	}

	if len(bOrd) > 0 {
		fmt.Fprintln(w, "\nRendering del corpo (ms)")
		fmt.Fprintln(tw, "fase\tn\tmediana\tp95\tmax\tKiB sorgente medi")
		for _, k := range bOrd {
			g := body[k]
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", k, len(g.dur),
				ms(median(g.dur)), ms(percentile(g.dur, 95)), ms(maxOf(g.dur)),
				kib(float64(sumOf(g.bytes))/float64(len(g.bytes))))
		}
		_ = tw.Flush()
	}
	if len(cacheOrder) > 0 {
		fmt.Fprintln(w, "\nCache (hit rate per tipo; eta mediana dell'istantanea mostrata)")
		fmt.Fprintln(tw, "tipo\thit\tmiss\tepoca\thit rate\teta mediana")
		for _, k := range cacheOrder {
			c := cacheOps[k]
			rate := "-"
			if tot := c.hit + c.miss; tot > 0 {
				rate = fmt.Sprintf("%.1f%%", 100*float64(c.hit)/float64(tot))
			}
			age := "-"
			if len(c.age) > 0 {
				age = secs(median(c.age))
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\t%s\n", k, c.hit, c.miss, c.epoch, rate, age)
		}
		_ = tw.Flush()
	}
	return nil
}
