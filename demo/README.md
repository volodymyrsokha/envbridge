# README demo

The GIF on the front page is recorded with [vhs](https://github.com/charmbracelet/vhs)
against a throwaway local SSH server — nothing real is touched, so anyone can
re-record it after CLI output changes:

```console
$ brew install vhs
$ vhs demo/demo.tape        # from the repo root
```

`setup.sh` builds envbridge and the fake server, seeds a demo project under
`/tmp/envbridge-demo`, and is invoked by the tape automatically (hidden from
the recording).
