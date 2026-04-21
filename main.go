package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

var exitCmdFlag = flag.String("c", "", "custom shell command to execute to kill the process. $TARGET_PID will be set to the child process ID")
var killTimeoutFlag = flag.Int("k", -1, "seconds to wait after sending SIGTERM (or running custom kill command) before sending a forceful SIGKILL")
var verboseFlag = flag.Bool("v", false, "verbose mode")

const (
	StateNone = 0
	StateOne  = 1
	StateTwo  = 2
)

type obs struct {
	last  time.Time
	state int
}

func (o *obs) observe() bool {
	res := false
	now := time.Now()
	withinTime := now.Sub(o.last) < 500*time.Millisecond

	switch o.state {
	case StateNone:
		o.state = StateOne
	case StateOne:
		if withinTime {
			o.state = StateTwo
		} else {
			o.state = StateOne
		}
	case StateTwo:
		if withinTime {
			o.state = StateNone
			res = true
		} else {
			o.state = StateOne
		}
	}

	o.last = now
	return res
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "No cmd provided")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, `Press ^] three times quickly to kill`)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	cmd := exec.Command(args[0], args[1:]...)

	ptmx, tty, err := pty.Open()
	if err != nil {
		panic(err)
	}
	defer func() { _ = ptmx.Close() }()

	ttyIfTerm := func(f *os.File) *os.File {
		if term.IsTerminal(int(f.Fd())) {
			return tty
		} else {
			return f
		}
	}

	cmd.Stdin = ttyIfTerm(os.Stdin)

	cmdStdout := ttyIfTerm(os.Stdout)
	cmdStderr := ttyIfTerm(os.Stderr)
	cmd.Stdout = cmdStdout
	cmd.Stderr = cmdStderr

	var ptmxTarget *os.File
	if cmdStdout == tty {
		ptmxTarget = os.Stdout
	} else {
		if cmdStderr == tty {
			ptmxTarget = os.Stderr
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	tty.Close()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.
	defer func() { signal.Stop(ch); close(ch) }()

	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		var o obs
		kills := 0
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if n == 0 {
				return
			}
			data := buf[:n]
			if bytes.IndexByte(data, 0x1d) != -1 {
				if o.observe() {
					if kills == 0 {
						if *exitCmdFlag != "" {
							if *verboseFlag {
								log.Printf(`Killing %d with custom command: "%s"`, cmd.Process.Pid, *exitCmdFlag)
							}
							exitCmd := exec.Command("bash", "-c", *exitCmdFlag)
							exitCmd.Env = exitCmd.Environ()
							exitCmd.Env = append(exitCmd.Env,
								fmt.Sprintf("TARGET_PID=%d", cmd.Process.Pid),
							)
							go func() {
								if out, err := exitCmd.CombinedOutput(); err != nil {
									log.Printf("Failed to run stop command: %v\n%s", err, string(out))
								}
							}()
						} else {
							if *verboseFlag {
								log.Printf(`Killing %d with SIGTERM`, cmd.Process.Pid)
							}
							cmd.Process.Signal(syscall.SIGTERM)
						}
						go func() {
							if *killTimeoutFlag >= 0 {
								time.Sleep(time.Duration(*killTimeoutFlag) * time.Second)
								if *verboseFlag {
									log.Printf(`Killing %d with SIGKILL after timeout`, cmd.Process.Pid)
								}
								cmd.Process.Signal(syscall.SIGKILL)

							}
						}()
					} else {
						if *verboseFlag {
							log.Printf("Killing %v with SIGKILL", cmd.Process.Pid)
						}
						cmd.Process.Signal(syscall.SIGKILL)
					}
					kills++
				}
			}

			nn, err := ptmx.Write(data)
			if err != nil {
				return
			}
			_ = nn
		}
	}()
	wg.Go(func() {
		if ptmxTarget != nil {
			n, err := io.Copy(ptmxTarget, ptmx)
			_, _ = n, err
		}
	})

	cmd.Process.Wait()

}
