# gmail-tui demo tape — regenerate the README GIF with:
#
#   vhs docs/demo.tape
#
# Requires: vhs (brew install vhs), Go 1.26+. The tape never touches the real
# config: it builds a throwaway binary and runs it with HOME pointed at a
# dedicated directory ($HOME/.local/share/vhs-home), wiped on every run.

Output docs/demo.gif

Require go

Set Shell bash
Set FontSize 17
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set Padding 28
Set TypingSpeed 25ms

Hide
Type "rm -rf $HOME/.local/share/vhs-home && mkdir -p $HOME/.local/share/vhs-home"
Enter
Type "go build -o /tmp/gmail-tui-demo ./cmd/gmail-tui"
Enter
Sleep 5s
Type "export HOME=$HOME/.local/share/vhs-home"
Enter
Type "clear"
Enter
Sleep 500ms
Show

Sleep 1500ms

# No config, no password: the first-run wizard greets us.
Type "/tmp/gmail-tui-demo"
Enter
Sleep 3500ms

# s skips the setup → demo mode with seeded mail and a banner.
Type "s"
Sleep 3000ms

# Browse the list a little (j/k).
Type "j"
Sleep 500ms
Type "k"
Sleep 700ms

# Enter opens the selected thread.
Enter
Sleep 2600ms

# :theme switches theme at runtime (immediate preview, persisted).
Type ":theme dracula"
Enter
Sleep 2800ms

Escape
Sleep 800ms

# Second thread, with an attachment: save it with :save.
Type "j"
Sleep 300ms
Type "j"
Sleep 500ms
Enter
Sleep 1500ms
Type ":save 1 /tmp/report-spese-q4.pdf"
Enter
Sleep 2600ms

Escape
Sleep 800ms

# Compose a new message (c): To → Tab → Subject.
Type "c"
Sleep 1500ms
Type "ada@example.com"
Sleep 400ms
Tab
Sleep 300ms
Type "Ciao da gmail-tui"
Sleep 400ms

# Ctrl+S asks for confirmation, s sends.
Ctrl+S
Sleep 2200ms
Type "s"
Sleep 3000ms

# Back in the inbox: quit.
Type "q"
Sleep 2000ms
