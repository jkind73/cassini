// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/jkind73/cassini/core"
	"github.com/jkind73/cassini/internal/debugserver"
	"github.com/jkind73/cassini/internal/replay"
	"github.com/user-none/eblitui/romloader"
	"golang.org/x/sys/windows"
)

func main() {
	biosPath := flag.String("bios", "", "Path to Saturn BIOS ROM (512KB). Optional - if omitted, the HLE BIOS boots the disc directly.")
	discPath := flag.String("disc", "", "Path to CHD V5 or cue disc image.")
	cpuProfile := flag.String("cpuprofile", "", "Write CPU profile to file.")
	dumpDir := flag.String("dump-dir", ".", "Directory to write memory dumps into (created if missing).")
	savePath := flag.String("save", "", "Path to backup-RAM save file. If a directory, uses <gameid>.srm inside it. Loaded on start (if it exists) and written on close.")
	fastBoot := flag.Bool("fast-boot", false, "Skip the real BIOS boot animation and enter the disc IP directly (real BIOS only).")
	record := flag.String("record", "", "Record a replay file (JSON) of per-frame input and screenshot markers.")
	replayPath := flag.String("replay", "", "Replay a recorded input file (JSON). Recorded input is mixed with live input so you can still press buttons.")
	loadState := flag.String("load-state", "", "Path to a save state file to load at startup. Requires the same disc and BIOS the state was captured with.")
	enableDebugServer := flag.Bool("debug-server", false, "Enable the debug server, bound to 127.0.0.1.")
	debugServerPort := flag.Int("debug-server-port", 5000, "Debug server TCP port.")
	launchDebugUI := flag.Bool("ui", false, "Launch debugger UI automatically (implies -debug-server).")
	flag.Parse()

	if *record != "" && *replayPath != "" {
		log.Fatal("-record and -replay are mutually exclusive")
	}

	var cpuProfileFile *os.File
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("failed to create CPU profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("failed to start CPU profile: %v", err)
		}
		cpuProfileFile = f
	}

	if *biosPath == "" && *discPath == "" {
		log.Printf("[INFO] No -bios or -disc specified; starting in HLE standalone mode.")
	}

	emu := core.NewEmulator()

	if *biosPath != "" {
		biosData, err := os.ReadFile(*biosPath)
		if err != nil {
			log.Fatalf("failed to read BIOS: %v", err)
		}
		if err := emu.SetBIOS("main_bios", biosData); err != nil {
			log.Fatalf("failed to set BIOS: %v", err)
		}
	}

	var disc *romloader.Disc
	var err error
	if *discPath != "" {
		disc, err = romloader.OpenDisc(*discPath)
		if err != nil {
			log.Fatalf("failed to open disc: %v", err)
		}
		printDiscInfo(disc)
		emu.SetDisc(disc)
	}

	resolvedSavePath := resolveSavePath(*savePath, disc)
	if resolvedSavePath != "" {
		loadSaveFile(emu, resolvedSavePath)
	}

	var recorder *replay.Recorder
	if *record != "" {
		recorder = replay.NewRecorder()
		fmt.Printf("[REPLAY] recording to %s\n", *record)
	}

	var player *replay.Player
	if *replayPath != "" {
		rf, err := replay.Load(*replayPath)
		if err != nil {
			log.Fatalf("failed to load replay file %q: %v", *replayPath, err)
		}
		if rf.Version != replay.Version {
			log.Printf("Warning: replay file version %d, tool expects %d", rf.Version, replay.Version)
		}
		if id := readGameID(disc); rf.DiscID != "" && id != "" && rf.DiscID != id {
			log.Printf("Warning: replay disc %q does not match loaded disc %q", rf.DiscID, id)
		}
		player = replay.NewPlayer(rf)
		fmt.Printf("[REPLAY] replaying %s (%d frames, %d screenshots)\n", *replayPath, rf.Frames, len(rf.Screenshots))
	}

	// SIGINT/SIGTERM handler. Flushes the save file (if -save was
	// given), the replay file (if -record was given), and stops the CPU
	// profile (if -cpuprofile was given) before exiting. Without this,
	// CTRL-C would skip all three — the normal close-path only runs when
	// ebiten exits cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		if resolvedSavePath != "" {
			writeSaveFile(emu, resolvedSavePath)
		}
		if recorder != nil {
			if err := recorder.Write(*record, readGameID(disc)); err != nil {
				log.Printf("Warning: failed to write replay file %q: %v", *record, err)
			}
		}
		if cpuProfileFile != nil {
			pprof.StopCPUProfile()
			cpuProfileFile.Close()
		}
		os.Exit(0)
	}()

	if *fastBoot {
		emu.SetOption("fast_boot", "true")
	}

	if err := emu.Start(); err != nil {
		log.Fatalf("emulator start failed: %v", err)
	}
	//emu.InstallPCWatchdog()

	// State load happens after Start so the boot path has run (HLE
	// service hooks wired, workers spawned but parked) and before the
	// first RunFrame, satisfying Deserialize's frame-boundary
	// constraint.
	if *loadState != "" {
		stateData, err := os.ReadFile(*loadState)
		if err != nil {
			log.Fatalf("failed to read save state %q: %v", *loadState, err)
		}
		if err := emu.Deserialize(stateData); err != nil {
			log.Fatalf("failed to load save state %q: %v", *loadState, err)
		}
		fmt.Printf("[STATE] loaded %s\n", *loadState)
	}

	log.Printf("[WINDOW] Configuring Ebiten window...")
	ebiten.SetWindowTitle("Saturn")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowSizeLimits(400, 300, -1, -1)
	log.Printf("[WINDOW] Window configured. Proceeding to audio and game init...")

	// Frame rate comes from the core's region (60 NTSC / 50 PAL), read once
	// after Start so the audio sizing and pacing match the loaded game.
	fps := emu.GetTiming().FPS

	log.Printf("[DEBUG] Initializing audio player with FPS: %d", fps)
	var ap *audioPlayer
	//var err error
	ap, err = newAudioPlayer(fps)
	if err != nil {
		log.Printf("Warning: audio initialization failed: %v", err)
	} else {
		log.Printf("[DEBUG] Audio player initialized successfully")
	}
	audioPlayer := ap

	g := &game{
		emu:         emu,
		audioPlayer: audioPlayer,
		fps:         fps,
		sharedInput: &sharedInput{},
		sharedFB:    newSharedFramebuffer(maxFBWidth, maxFBHeight),
		control:     newEmuControl(),
		emuDone:     make(chan struct{}),
		keyMap:      buildKeyMap(),
		padMap:      buildPadMap(),
		dumpDir:     *dumpDir,
		recorder:    recorder,
		player:      player,
	}

	// Enable debug server if -ui flag is set
	debugServerEnabled := *enableDebugServer || *launchDebugUI
	if debugServerEnabled {
		if *debugServerPort < 1 || *debugServerPort > 65535 {
			log.Fatalf("debug server port must be 1-65535")
		}
		log.Printf("[DEBUG] Starting debug server on port %d...", *debugServerPort)
		s, err := debugserver.Start(*debugServerPort, emu, &g.paused)
		if err != nil {
			log.Fatalf("failed to start debug server: %v", err)
		}
		log.Printf("[DEBUG] Debug server started successfully")
		g.debugServer = s
		fmt.Printf("[DEBUGSERVER] listening on 127.0.0.1:%d\n", *debugServerPort)

		// Launch debugger UI if requested
		if *launchDebugUI {
			log.Printf("[DEBUG] Launching debugger client...")
			go launchDebuggerClient(*debugServerPort)
		}
	}

	log.Printf("[DEBUG] Starting watchdog and emulation loop...")
	g.startWatchdog()
	go g.emulationLoop()
	log.Printf("[DEBUG] Watchdog and loop started")

	log.Printf("[WINDOW] Attempting to launch Ebiten window with RunGame...")
	runErr := ebiten.RunGame(g)

	// Always run the close path on any clean exit (Cmd+Q, window-X,
	// ebiten.Termination). On macOS Cmd+Q goes through AppKit and
	// does NOT raise a Unix signal, so the SIGINT goroutine never
	// fires for those paths — the save flush + pprof flush + disc
	// release have to happen here.
	g.close()
	if resolvedSavePath != "" {
		writeSaveFile(emu, resolvedSavePath)
	}
	if recorder != nil {
		if err := recorder.Write(*record, readGameID(disc)); err != nil {
			log.Printf("Warning: failed to write replay file %q: %v", *record, err)
		}
	}
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		cpuProfileFile.Close()
	}
	if disc != nil {
		disc.Close()
	}

	// ebiten.Termination is the sentinel ebiten returns for a normal
	// window close — not a fatal error. Only escalate other errors.
	if runErr != nil && !errors.Is(runErr, ebiten.Termination) {
		log.Fatal(runErr)
	}
}

// launchDebuggerClient launches the debugger UI tool as a subprocess.
// It runs in a goroutine to not block the main thread.
func launchDebuggerClient(port int) {
	// Give the debug server a moment to start up
	time.Sleep(1 * time.Second)

	// Get the current working directory (workspace root)
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("[DEBUGGER] Failed to get working directory: %v", err)
		return
	}

	// Find the debugger tool in the utils/debugger directory
	debuggerDir := filepath.Join(wd, "utils", "debugger")

	// On Windows, we need to build the debugger first to get a proper .exe
	// that can create a GUI window independently
	debuggerExe := filepath.Join(debuggerDir, "debugger.exe")

	log.Printf("[DEBUGGER] Building debugger at %s", debuggerExe)

	// Build the debugger executable
	buildCmd := exec.Command("go", "build", "-o", "debugger.exe", ".")
	buildCmd.Dir = debuggerDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		log.Printf("[DEBUGGER] Failed to build debugger: %v", err)
		// Fall back to go run if build fails
		launchDebuggerWithGoRun(debuggerDir, port)
		return
	}

	// Launch the built executable as a separate process
	debuggerCmd := exec.Command(debuggerExe, "-connect", fmt.Sprintf("127.0.0.1:%d", port))
	debuggerCmd.Dir = debuggerDir
	// Don't attach stdout/stderr to parent process - let it have its own console
	debuggerCmd.Stdout = nil
	debuggerCmd.Stderr = nil
	// On Windows, set CREATE_NEW_CONSOLE to get a separate window
	debuggerCmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}

	log.Printf("[DEBUGGER] Launching debugger UI: %s -connect 127.0.0.1:%d", debuggerExe, port)

	// Start the debugger process (non-blocking)
	if err := debuggerCmd.Start(); err != nil {
		log.Printf("[DEBUGGER] Failed to start debugger: %v", err)
		// Fall back to go run
		launchDebuggerWithGoRun(debuggerDir, port)
		return
	}

	// Detach from the child process - let it run independently
	go func() {
		if err := debuggerCmd.Wait(); err != nil {
			log.Printf("[DEBUGGER] Debugger process ended: %v", err)
		}
	}()
}

// launchDebuggerWithGoRun is a fallback using go run if building fails
func launchDebuggerWithGoRun(debuggerDir string, port int) {
	log.Printf("[DEBUGGER] Falling back to go run in %s", debuggerDir)

	// Use start command on Windows to create a new console window
	cmdLine := fmt.Sprintf("go run . -connect 127.0.0.1:%d", port)

	// On Windows, use cmd /c start to open a new window
	debuggerCmd := exec.Command("cmd", "/c", "start", "cmd", "/c", cmdLine)
	debuggerCmd.Dir = debuggerDir
	debuggerCmd.Stdout = nil
	debuggerCmd.Stderr = nil

	if err := debuggerCmd.Start(); err != nil {
		log.Printf("[DEBUGGER] Failed to start debugger with go run: %v", err)
	}
}
