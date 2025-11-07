// Package main is the main entry point for the application.
package main

import (
	"github.com/fishnix/golang-template/cmd"
)

// //go:embed static
// var static embed.FS

// //go:embed templates
// var templates embed.FS

func main() {
	// // pass down the embedded filesystem to the cmd package
	// cmd.Static = static
	// cmd.Templates = templates
	cmd.Execute()
}
