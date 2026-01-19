package main

// main is the entry point of the watchdog application.
// It initializes and executes the Cobra CLI command structure defined in root.go.
// main is the program entry point. It delegates initialization and execution to Execute,
// which sets up the Cobra CLI command structure (defined in root.go) and runs the application's logic.
func main() {
	Execute()
}