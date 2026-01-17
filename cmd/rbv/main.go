package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"radiobuenavia/internal/app"
	"radiobuenavia/internal/config"
)

const art = `
You are watching... Radio Buena Via...
           _ . - = - . _
       . "  \  \   /  /  " .
     ,  \                 /  .
   . \   _,.--~=~"~=~--.._   / .
  ;  _.-"  / \ !   ! / \  "-._  .
/ ,"     / ,' .---. ', \     ". \
/.'   '~  |   /:::::\   |  ~'   '.\
\'.  '~   |   \:::::/   | ~'  ~ .'\
 \ '.  '~ \ ', '~~~' ,' /   ~'.' /
  .  "-._  \ / !   ! \ /  _.-"  .
   ./    "=~~.._  _..~~='"    \.
     ,/         ""          \,
       . _/             \_ .
          " - ./. .\. - "
`

func main() {
	exitCode := 0
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "init":
			runInit(args[1:])
			return
		case "doctor":
			runDoctor(args[1:])
			return
		}
	}

	fs := flag.NewFlagSet("rbv", flag.ExitOnError)
	configPath := fs.String("config", "./config.toml", "path to config file")
	pause := fs.Bool("pause", defaultPause(), "pause before exit")
	_ = fs.Parse(args)

	if err := runMain(*configPath); err != nil {
		log.Print(err)
		exitCode = 1
	}
	pauseIfRequested(*pause)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func runMain(configPath string) error {
	log.Print("This software is distributed under the GNU GENERAL PUBLIC LICENSE agreement.")
	log.Print("This software comes with absolutely no warranty or liability.")
	log.Print("More information can be found in the LICENSE file.")
	log.Print(art)

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	app := app.New(cfg)
	if err := app.Run(); err != nil {
		return fmt.Errorf("run failed: %w", err)
	}
	return nil
}

func defaultPause() bool {
	return runtime.GOOS == "windows"
}

func pauseIfRequested(pause bool) {
	if !pause {
		return
	}
	fmt.Print("Press Enter to close...")
	_, _ = fmt.Fscanln(os.Stdin)
}
