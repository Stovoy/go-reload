package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/howeyc/fsnotify"
)

type Options struct {
	Verbose bool
}

var fileWatcher *fsnotify.Watcher
var watchedDirs map[string]struct{}
var ticker *time.Ticker
var cmd *exec.Cmd
var run func() *exec.Cmd
var options Options

func main() {
	// Check args length
	if len(os.Args) == 1 {
		fmt.Println("Must have at least one file.")
		return
	}
	watchedDirs = make(map[string]struct{})

	var filesArgsIndex int

	options = Options{Verbose: false}
	// Get option flags.
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-v" {
			options.Verbose = true
		} else {
			filesArgsIndex = i
			break
		}
	}

	if filesArgsIndex == len(os.Args) {
		fmt.Println("Must have at least one file.")
		return
	}

	// Create run function, and run once.
	run = makeRun(os.Args[filesArgsIndex:])
	cmd = run()

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Fatal error: ", err)
		return
	}
	fileWatcher = watcher

	// Create a ticker to check the file events that have occurred.
	ticker = time.NewTicker(3 * time.Second)
	go processChanges()

	// Begin watching root directory.
	err = filepath.Walk(".", visit)
	if err != nil {
		fmt.Println("Error: ", err)
	}

	// Block in main.
	<-make(chan struct{})
}

func makeRun(files []string) func() *exec.Cmd {
	return func() *exec.Cmd {
		fmt.Printf("Running [%s]...\n", time.Now().Format("02 Jan 06 15:04:05"))

		args := append([]string{"run"}, files...)
		cmd := exec.Command("go", args...)

		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		cmd.Start()

		return cmd
	}
}

func processChanges() {
	for {
		select {
		case <-ticker.C:
			// Store a count of changes that have occurred.
			count := 0
			done := false
			for {
				select {
				case e := <-fileWatcher.Event:
					if e.IsCreate() {
						watchIfDir(e.Name)
					} else if e.IsDelete() {
						if _, has := watchedDirs[e.Name]; has {
							// If an existing directory was removed, close the watcher on it.
							delete(watchedDirs, e.Name)
							fileWatcher.RemoveWatch(e.Name)
						}
					} else if e.IsRename() {
						if _, has := watchedDirs[e.Name]; has {
							// If an existing directory was renamed, close the watcher on it.
							delete(watchedDirs, e.Name)
							fileWatcher.RemoveWatch(e.Name)
						} else {
							watchIfDir(e.Name)
						}
					}
					// Keep track of how many events occurred
					count++
				case err := <-fileWatcher.Error:
					fmt.Println("Error: ", err)
				default:
					done = true
				}
				if done {
					// No more data in the chan.
					break
				}
			}
			if count == 0 {
				// No events occurred, no need to reload.
				break
			}
			cmd.Process.Kill()
			cmd = run()
		}
	}
}

func visit(path string, f os.FileInfo, _ error) error {
	if f.IsDir() {
		watchDir(path)
	}

	return nil
}

func watchIfDir(path string) {
	// If a new directory was created, open a watcher on it.
	fileInfo, err := os.Stat(path)
	if err != nil {
		if options.Verbose {
			// Only show in verbose mode, because some editors use temp files
			// which will throw errors here
			fmt.Println("Error: ", err)
		}
	} else if fileInfo.IsDir() {
		watchDir(path)
	}
}

func watchDir(path string) {
	watchedDirs[path] = struct{}{}
	fileWatcher.Watch(path)
}
