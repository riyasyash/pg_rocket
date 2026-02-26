package extractor

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

// ProgressTracker provides colored output and progress reporting for
// data extraction operations. It displays phase information, table discovery,
// progress bars, and statistics.
type ProgressTracker struct {
	verbose      bool
	startTime    time.Time
	currentPhase string
	bar          *progressbar.ProgressBar

	// Terminal color formatters
	cyan    *color.Color
	green   *color.Color
	yellow  *color.Color
	blue    *color.Color
	magenta *color.Color
}

// NewProgressTracker creates a new progress tracker.
// If verbose is true, detailed logging is enabled.
func NewProgressTracker(verbose bool) *ProgressTracker {
	return &ProgressTracker{
		verbose:   verbose,
		startTime: time.Now(),
		cyan:      color.New(color.FgCyan, color.Bold),
		green:     color.New(color.FgGreen, color.Bold),
		yellow:    color.New(color.FgYellow, color.Bold),
		blue:      color.New(color.FgBlue),
		magenta:   color.New(color.FgMagenta),
	}
}

func (pt *ProgressTracker) StartPhase(phase string) {
	pt.currentPhase = phase
	if pt.verbose {
		pt.cyan.Printf("\n🚀 %s\n", phase)
		fmt.Println(color.New(color.FgHiBlack).Sprint("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
	}
}

func (pt *ProgressTracker) Info(format string, args ...interface{}) {
	if pt.verbose {
		pt.blue.Printf("   ℹ  "+format+"\n", args...)
	}
}

func (pt *ProgressTracker) Success(format string, args ...interface{}) {
	if pt.verbose {
		pt.green.Printf("   ✓  "+format+"\n", args...)
	}
}

func (pt *ProgressTracker) Warning(format string, args ...interface{}) {
	if pt.verbose {
		pt.yellow.Printf("   ⚠  "+format+"\n", args...)
	}
}

func (pt *ProgressTracker) Progress(current, total int, description string) {
	if !pt.verbose {
		return
	}

	if pt.bar == nil || total != pt.bar.GetMax() {
		pt.bar = progressbar.NewOptions(total,
			progressbar.OptionSetDescription(description),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "█",
				SaucerHead:    "█",
				SaucerPadding: "░",
				BarStart:      "[",
				BarEnd:        "]",
			}),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("rows"),
		)
	}
	pt.bar.Set(current)
}

func (pt *ProgressTracker) FinishProgress() {
	if pt.bar != nil {
		pt.bar.Finish()
		fmt.Fprintln(os.Stderr)
		pt.bar = nil
	}
}

func (pt *ProgressTracker) TableDiscovered(tableName string, rowCount int) {
	if pt.verbose {
		pt.magenta.Printf("   📦 Found %d rows in %s\n", rowCount, tableName)
	}
}

func (pt *ProgressTracker) TraversingTable(tableName string, direction string) {
	if pt.verbose {
		arrow := "⬆️"
		if direction == "down" {
			arrow = "⬇️"
		}
		pt.Info("%s Traversing %s: %s", arrow, direction, tableName)
	}
}

func (pt *ProgressTracker) Complete(totalRows, totalTables int) {
	elapsed := time.Since(pt.startTime)

	if pt.verbose {
		fmt.Println()
		pt.cyan.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		pt.green.Printf("✓ Extraction Complete!\n\n")

		fmt.Printf("  📊 Statistics:\n")
		pt.blue.Printf("     • Total rows:   %d\n", totalRows)
		pt.blue.Printf("     • Total tables: %d\n", totalTables)
		pt.blue.Printf("     • Time taken:  %v\n", elapsed.Round(time.Millisecond))

		if totalRows > 0 {
			rowsPerSec := float64(totalRows) / elapsed.Seconds()
			pt.blue.Printf("     • Speed:       %.0f rows/sec\n", rowsPerSec)
		}

		fmt.Println()
	} else {
		// Even in non-verbose mode, show a simple summary
		pt.green.Printf("✓ Extracted %d rows from %d tables in %v\n",
			totalRows, totalTables, elapsed.Round(time.Millisecond))
	}
}

func (pt *ProgressTracker) OutputGeneration(format string) {
	if pt.verbose {
		pt.StartPhase(fmt.Sprintf("Generating %s output", format))
	}
}

func (pt *ProgressTracker) WritingTable(tableName string, rowCount int, current, total int) {
	if pt.verbose {
		pt.Progress(current, total, fmt.Sprintf("Writing %s (%d rows)", tableName, rowCount))
	}
}
