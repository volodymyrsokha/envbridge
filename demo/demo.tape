# README demo — record from the repo root with: vhs demo/demo.tape
# Runs against the self-contained fake server from demo/setup.sh; nothing
# real is touched.

Output demo/demo.gif

Set Shell bash
Set FontSize 15
Set Width 1100
Set Height 620
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Hide
Type "sh demo/setup.sh && source /tmp/envbridge-demo/env.sh && clear"
Enter
Sleep 12s
Show

Type "envbridge status"
Sleep 400ms
Enter
Sleep 3s

Type "echo 'FEATURE_CHECKOUT_V2=on' >> .env.production"
Sleep 300ms
Enter
Sleep 1.2s

Type "envbridge diff production"
Sleep 400ms
Enter
Sleep 3.5s

Type "envbridge push production"
Sleep 400ms
Enter
Sleep 3s

Type "# meanwhile, someone hand-edits prod at 2 a.m. ..."
Sleep 300ms
Enter
Type "echo 'HOTFIX_MODE=1' >> ~/app/.env"
Sleep 300ms
Enter
Sleep 1.5s

Type "envbridge status"
Sleep 400ms
Enter
Sleep 3s

Type "envbridge push production"
Sleep 400ms
Enter
Sleep 3.5s

Type "envbridge pull production --adopt"
Sleep 400ms
Enter
Sleep 3s

Type "envbridge status"
Sleep 400ms
Enter
Sleep 5s

Hide
Type "kill $(cat /tmp/envbridge-demo/server.pid)"
Enter
