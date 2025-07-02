//go:build !windows
// +build !windows

package pkg

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// PBar is the progress bar model
type PBar struct {
	Total            uint16         // Total number of iterations to sum 100%
	CurrentIteration int            // Current iteration
	header           uint16         // Header length, to be used to calculate the bar width "Progress: [100%] []"
	wscol            uint16         // Window width
	wsrow            uint16         // Window height
	doneStr          string         // Progress bar done string
	ongoingStr       string         // Progress bar ongoing string
	signalWinch      chan os.Signal // Signal handler: SIGWINCH
	signalTerm       chan os.Signal // Signal handler: SIGTERM
	once             sync.Once      // Close the signal channel only once
	winSize          struct {       // winSize is the struct to store the current window size, used by ioctl
		Row    uint16 // row
		Col    uint16 // column
		Xpixel uint16 // X pixel
		Ypixel uint16 // Y pixel
	}
}

// NewPBar create a new progress bar
// After NewPBar() is called:
//   - initialize SignalHandler()
//   - update pBar.Total for new number of iterations to sum 100%
//
// After progressBar() is finished:
//   - do a CleanUp()
func NewPBar() *PBar {
	pb := &PBar{
		Total:            0,
		CurrentIteration: 0,
		header:           0,
		wscol:            0,
		wsrow:            0,
		doneStr:          "#",
		ongoingStr:       ".",
		signalWinch:      make(chan os.Signal, 1),
		signalTerm:       make(chan os.Signal, 1),
	}

	signal.Notify(pb.signalWinch, syscall.SIGWINCH) // Register SIGWINCH signal
	signal.Notify(pb.signalTerm, syscall.SIGTERM)   // Register SIGTERM signal

	_ = pb.UpdateWSize()

	pb.SignalHandler()

	return pb
}

func (pb *PBar) computeHeader() {
	switch {
	case pb.wscol <= 9:
		pb.header = uint16(len("[100%]"))
	case pb.wscol <= 20:
		pb.header = uint16(len("[100%] []"))
	default:
		// Dynamically calculate header length
		iterStr := fmt.Sprintf("[%d/%d] []", pb.CurrentIteration, pb.Total)
		pb.header = uint16(len(iterStr))
	}
}

// CleanUp restore reserved bottom line and restore cursor position
func (pb *PBar) CleanUp() {
	pb.once.Do(func() { close(pb.signalWinch) }) // Close the signal channel politely, avoid closing it twice
	pb.once.Do(func() { close(pb.signalTerm) })  // Close the signal channel politely

	if pb.winSize.Col == 0 || pb.winSize.Row == 0 {
		return // Not a terminal, running in a pipeline or test
	}

	fmt.Print("\x1B7")                 // Save the cursor position
	fmt.Printf("\x1B[0;%dr", pb.wsrow) // Drop margin reservation
	fmt.Printf("\x1B[%d;0f", pb.wsrow) // Move the cursor to the bottom line
	fmt.Print("\x1B[0K")               // Erase the entire line
	fmt.Print("\x1B8")                 // Restore the cursor position util new size is calculated
	fmt.Print("\x1B[1J")
	fmt.Print("\x1B[2J")
}

// updateWSize update the window size
func (pb *PBar) UpdateWSize() error {
	isTerminal, err := pb.checkIsTerminal()
	if err != nil {
		return fmt.Errorf("could not check if the current process is running in a terminal: %w", err)
	}
	if !isTerminal {
		return nil // Not a terminal, running in a pipeline or test
	}
	//if pb.Total == uint16(100) {
	//	return nil // No need to update the header length
	//}

	pb.wscol = pb.winSize.Col
	pb.wsrow = pb.winSize.Row

	pb.computeHeader()

	fmt.Print("\x1BD")                   // Return carriage
	fmt.Print("\x1B7")                   // Save the cursor position
	fmt.Printf("\x1B[0;%dr", pb.wsrow-1) // Reserve the bottom line
	fmt.Print("\x1B8")                   // Restore the cursor position
	fmt.Print("\x1B[1A")                 // Moves cursor up # lines

	return nil
}

// Update increases the current iteration by one and re-renders
func (pb *PBar) Update() {
	pb.RenderPBar(pb.CurrentIteration)
	pb.CurrentIteration++
}

// UpdateWithTotal sets a new total and optionally resets progress
func (pb *PBar) UpdateWithTotal(newTotal int) {
	if newTotal <= 0 {
		pb.CleanUp()
		return
	}

	oldTotal := pb.Total
	pb.Total = uint16(newTotal)

	if pb.CurrentIteration > int(pb.Total) {
		pb.CurrentIteration = int(pb.Total)
	}

	// Recalculate header size on size change
	if oldTotal != pb.Total {
		_ = pb.UpdateWSize()
	}

	pb.Update()
}

func (pb *PBar) UpdateTotal(newTotal int) {
	if newTotal <= 0 {
		pb.CleanUp()
		return
	}

	oldTotal := pb.Total
	pb.Total = uint16(newTotal)

	if pb.CurrentIteration > int(pb.Total) {
		pb.CurrentIteration = int(pb.Total)
	}

	// Recalculate header size on size change
	if oldTotal != pb.Total {
		_ = pb.UpdateWSize()
		pb.computeHeader()
	}

}

// RenderPBar render the progress bar
func (pb *PBar) RenderPBar(count int) {
	if pb.winSize.Col == 0 || pb.winSize.Row == 0 {
		return // Not a terminal, running in a pipeline or test
	}

	pb.computeHeader()

	fmt.Print("\x1B7")       // Save the cursor position
	fmt.Print("\x1B[2K")     // Erase the entire line
	defer fmt.Print("\x1B8") // Restore the cursor position

	// Safely handle zero total
	if pb.Total == 0 {
		fmt.Printf("\x1B[%d;0H[\x1B[33m--%%\x1B[0m] [waiting...]", pb.wsrow)
		return
	}

	barWidth := int(math.Max(float64(pb.wscol-pb.header), 0))
	barDone := int(float64(barWidth) * float64(count) / float64(pb.Total))
	barDone = int(math.Min(float64(barDone), float64(barWidth)))

	done := strings.Repeat(pb.doneStr, barDone)
	todo := strings.Repeat(pb.ongoingStr, barWidth-barDone)
	bar := fmt.Sprintf("[%s%s]", done, todo)

	fmt.Printf("\x1B[%d;0H", pb.wsrow) // move cursor to row #, col #

	switch {
	case pb.wscol <= 9:
		fmt.Printf("[\x1B[33m%3d%%\x1B[0m]", uint16(count)*100/pb.Total)
	case pb.wscol <= 20:
		fmt.Printf("[\x1B[33m%3d%%\x1B[0m] %s", uint16(count)*100/pb.Total, bar)
	default:
		fmt.Printf("[\x1B[33m%d/%d\x1B[0m] %s", count, pb.Total, bar)
	}
}

// SignalHandler handle the signals, like SIGWINCH and SIGTERM
func (pb *PBar) SignalHandler() {
	go func() {
		for {
			select {
			case <-pb.signalWinch:
				if err := pb.UpdateWSize(); err != nil {
					panic(err) // The window size could not be updated
				}
			case <-pb.signalTerm:
				pb.CleanUp() // Restore reserved bottom line
				os.Exit(1)   // Exit gracefully but exit code 1
			}
		}
	}()
}

// checkIsTerminal check if the current process is running in a terminal
func (pb *PBar) checkIsTerminal() (isTerminal bool, err error) {
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&pb.winSize))); err != 0 {
		if err == syscall.ENOTTY || err == syscall.ENODEV {
			return false, nil // Not a terminal, running in a pipeline or test
		} else {
			return false, err // Other error
		}
	}

	return true, nil
}
