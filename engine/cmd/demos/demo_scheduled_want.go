package demos_moved

import (
	"context"
	"fmt"
	"time"

	mywant "mywant/engine/src"
)

// LoggerWant is a simple want that logs execution attempts
type LoggerWant struct {
	mywant.Want
}

// Progress is called during want execution - it logs the current execution
func (l *LoggerWant) Progress() {
	count := l.IncrementIntStateValue("execution_count", 0)
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] Scheduled execution #%d for Want '%s'\n",
		timestamp, count, l.Metadata.Name)
	l.StoreState("last_execution", timestamp)
}

// IsAchieved returns false so the want keeps running
func (l *LoggerWant) IsAchieved() bool {
	return false // Continuous execution
}

// Initialize resets state before execution begins
func (l *LoggerWant) Initialize() {
	// No initialization needed for logger
}

// NewLoggerWant creates a new logger want instance
func NewLoggerWant(m mywant.Metadata, s mywant.WantSpec) mywant.Progressable {
	return &LoggerWant{Want: *mywant.NewWantWithLocals(m, s, nil, "logger")}
}

func main() {
	fmt.Println("=== Want Scheduling Demo ===")
	fmt.Println("Demonstrates scheduled execution of wants")
	fmt.Println()

	// Create a config with a scheduled logger want
	config := mywant.Config{
		Wants: []*mywant.Want{
			{
				Metadata: mywant.Metadata{
					Name: "scheduled-logger",
					Type: "logger",
					Labels: map[string]string{
						"demo": "scheduled",
					},
				},
				Spec: mywant.WantSpec{
					Params: map[string]any{},
					// Schedule: Execute every 3 seconds (for demo purposes)
					When: []mywant.WhenSpec{
						{Every: "3 seconds"},
					},
				},
			},
		},
	}

	// Create chain builder
	builder := mywant.NewChainBuilderWithPaths("", "")
	builder.SetConfigInternal(config)

	// Register logger want type
	builder.RegisterWantType("logger", NewLoggerWant)

	// Register scheduler want type
	mywant.RegisterSchedulerWantTypes(builder)

	// Start execution
	fmt.Println("Starting execution...")
	fmt.Println("Logger want will execute every 3 seconds")
	fmt.Println("Scheduler Want will manage the scheduling automatically")
	fmt.Println()

	builder.ExecuteWithMode(false) // Batch mode

	// Run for 15 seconds to see multiple executions
	fmt.Println("\n=== Running for 15 seconds ===")
	time.Sleep(15 * time.Second)

	builder.Stop()
	fmt.Println("\n=== Demo completed ===")
}
