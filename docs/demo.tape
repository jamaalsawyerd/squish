# Records the README demo. Run it with: make demo

Output docs/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 720
Set Padding 12
Set Framerate 20
Set TypingSpeed 60ms
Set Theme "Catppuccin Mocha"

Hide
Type "export PATH=$PWD/bin:$PATH XDG_CONFIG_HOME=$(mktemp -d) && rm -f samples/*-squished* && cd samples && clear"
Enter
Sleep 300ms
Show

# Fewer workers keep the press on screen. It is usually much quicker.
Type "squish --workers 2"
Sleep 400ms
Enter
Sleep 2s

# Pick some photos.
Down
Sleep 300ms
Down
Sleep 400ms
Down
Sleep 700ms
Space
Sleep 500ms
Type "a"
Sleep 900ms

# Tune them, and watch the estimate change.
Tab
Sleep 1800ms
Down
Sleep 400ms
Right
Sleep 800ms
Right
Sleep 1200ms
Left
Sleep 300ms
Left
Sleep 900ms

# Squish!
Enter
Sleep 9s

# Bask in it.
Sleep 3s
Down
Sleep 300ms
Down
Sleep 700ms
Type "q"
Sleep 1500ms
