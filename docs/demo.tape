# Record the README demo with https://github.com/charmbracelet/vhs
#   vhs docs/demo.tape
Output docs/demo.gif
Set Theme "Catppuccin Mocha"
Set FontSize 15
Set Width 1300
Set Height 760
Set TypingSpeed 45ms

Type "fomocli --demo lb" Enter
Sleep 2.5s
Type "clear && fomocli --demo trader CryptoKaleo" Enter
Sleep 3s
Type "clear && fomocli --demo t CryptoKaleo balances" Enter
Sleep 3s
Type "clear && fomocli --demo live" Enter
Sleep 5s
Ctrl+C
Type "clear && fomocli --demo dash" Enter
Sleep 2s
Down Down Enter
Sleep 2s
Type "3"
Sleep 3s
Type "q"
