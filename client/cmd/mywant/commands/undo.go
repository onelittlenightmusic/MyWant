package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Taking the last thing back.
//
// A board arranged by talking is a board arranged by something that mishears.
// "荻窪を右に" lands on 荻窪駅前 instead, and the only way back was to know
// where the tile had been standing before — which nobody does once it has
// moved. Without a way to undo, every spoken change has to be thought about
// before it is said, which is not how anybody talks.
//
// So the commands that rearrange the canvas write down how to put things back,
// and this runs the last one:
//
//	mywant undo          take the last change back
//	mywant undo --list   what could be taken back, newest first
//
// Only what can honestly be reversed is recorded. Moving a tile, pinning one,
// creating a want: each of those has an opposite, and it is written down at the
// moment the original is done, when the "before" is still known. Deleting a
// want has no opposite — a deleted want is gone — and nothing here pretends
// otherwise; that is what the confirmation before a destructive command is for.

// undoEntry is one change, and how to take it back.
type undoEntry struct {
	At   time.Time `json:"at"`
	What string    `json:"what"` // what was done, in words
	Undo []string  `json:"undo"` // the arguments that reverse it
}

// undoDepth is how many changes are remembered. Deep enough to walk back out of
// a misunderstanding, shallow enough that the file stays a file.
const undoDepth = 50

func undoPath() string { return filepath.Join(getMyWantDir(), "undo.json") }

func readUndoLog() []undoEntry {
	data, err := os.ReadFile(undoPath())
	if err != nil {
		return nil
	}
	var entries []undoEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return entries
}

func writeUndoLog(entries []undoEntry) {
	if len(entries) > undoDepth {
		entries = entries[len(entries)-undoDepth:]
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	// A failed write is not worth interrupting the change that just succeeded:
	// the user asked for the change, not for the bookkeeping.
	_ = os.WriteFile(undoPath(), data, 0o644)
}

// recordUndo remembers how to reverse what was just done.
//
// Called after the change, not before: a change that failed is not one anybody
// needs to take back, and recording it first would offer to undo something that
// never happened.
func recordUndo(what string, undo ...string) {
	// Undoing is not a change to be undone. The reversal runs as this same
	// binary (see UndoCmd), so without this the pin that puts a tile back would
	// record a pin of its own and the two would take turns forever — and the
	// entry being retired would race with the one being added.
	if os.Getenv(undoInProgressEnv) != "" {
		return
	}
	writeUndoLog(append(readUndoLog(), undoEntry{At: time.Now(), What: what, Undo: undo}))
}

// undoInProgressEnv marks the child process that is carrying out a reversal.
const undoInProgressEnv = "MYWANT_UNDOING"

var UndoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Take back the last change to the board",
	Long: `Reverses the most recent change that recorded how to reverse itself.

Moving, pinning and unpinning a tile, and creating a want, are all recorded when
they happen — with the cell the tile was standing on before, which is the part
nobody remembers afterwards. Deleting is not: a deleted want cannot be brought
back, and this does not pretend it can.`,
	Example: `  mywant undo
  mywant undo --list`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		entries := readUndoLog()
		if list, _ := cmd.Flags().GetBool("list"); list {
			if len(entries) == 0 {
				fmt.Println("Nothing to undo.")
				return
			}
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				fmt.Printf("%s  %s  (undo: mywant %s)\n",
					e.At.Local().Format("15:04:05"), e.What, strings.Join(e.Undo, " "))
			}
			return
		}
		if len(entries) == 0 {
			fmt.Println("Nothing to undo.")
			return
		}

		last := entries[len(entries)-1]
		self, err := os.Executable()
		if err != nil {
			self = os.Args[0]
		}
		// Run through this same binary so the reversal is the command a person
		// would have typed — one implementation of "pin a thing", not two.
		reversal := exec.Command(self, last.Undo...)
		reversal.Env = append(os.Environ(), undoInProgressEnv+"=1")
		out, err := reversal.CombinedOutput()
		fmt.Print(string(out))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not undo %q: %v\n", last.What, err)
			os.Exit(1)
		}
		// Dropped only once it worked, so a failed undo can be tried again.
		writeUndoLog(entries[:len(entries)-1])
		fmt.Printf("Undid: %s\n", last.What)
	},
}

func init() {
	UndoCmd.Flags().Bool("list", false, "Show what could be undone, newest first")
}
