// Package climd provides a lightweight command-line interface builder
// as a simpler alternative to Cobra with minimal setup overhead.
package climd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Flag represents a command flag
type Flag struct {
	Name     string
	Short    string
	Value    string
	Usage    string
	Required bool
}

// SubCommand represents a subcommand (for commands like db that have sub-actions)
type SubCommand struct {
	Name  string
	Short string
	Long  string
	Flags []Flag
	Run   func(ctx context.Context, args []string, flags map[string]string) error
}

// Command represents a command in the CLI application
type Command struct {
	Name        string
	Short       string
	Long        string
	Flags       []Flag
	SubCommands []SubCommand // For commands that have sub-actions like 'db init', 'db migrate'
	Run         func(ctx context.Context, args []string, flags map[string]string) error
}

// AppConfig holds the configuration for the CLI application
type AppConfig struct {
	Name        string
	Version     string
	Description string
	Commands    []Command
}

// Run executes the application with the given arguments
func Run(config AppConfig, args []string) error {
	if len(args) == 0 {
		args = os.Args[1:]
	}

	ctx := context.Background()
	
	// Handle global flags
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h":
			printHelp(config)
			return nil
		case "--version", "-v":
			fmt.Printf("%s version %s\n", config.Name, config.Version)
			return nil
		}
	}

	// If no commands defined, print help
	if len(config.Commands) == 0 {
		printHelp(config)
		return nil
	}

	// Find and execute the appropriate command
	if len(args) == 0 {
		printHelp(config)
		return nil
	}

	cmdName := args[0]
	for _, cmd := range config.Commands {
		if cmd.Name == cmdName {
			// Check if this command has subcommands
			if len(cmd.SubCommands) > 0 && len(args) > 1 {
				subCmdName := args[1]
				for _, subCmd := range cmd.SubCommands {
					if subCmd.Name == subCmdName {
						// Parse flags and args for this subcommand
						cmdArgs, cmdFlags := parseArgs(args[2:], subCmd.Flags)
						
						// Check required flags
						for _, flag := range subCmd.Flags {
							if flag.Required {
								if _, exists := cmdFlags[flag.Name]; !exists {
									if flag.Short != "" {
										if _, exists := cmdFlags[flag.Short]; !exists {
											return fmt.Errorf("required flag --%s or -%s not provided", flag.Name, flag.Short)
										}
									} else {
										return fmt.Errorf("required flag --%s not provided", flag.Name)
									}
								}
							}
						}
						
						return subCmd.Run(ctx, cmdArgs, cmdFlags)
					}
				}
				
				// Subcommand not found
				fmt.Printf("Unknown subcommand: %s for command %s\n", subCmdName, cmdName)
				printCommandHelp(cmd)
				return fmt.Errorf("unknown subcommand: %s", subCmdName)
			} else {
				// Parse flags and args for this command
				cmdArgs, cmdFlags := parseArgs(args[1:], cmd.Flags)
				
				// Check required flags
				for _, flag := range cmd.Flags {
					if flag.Required {
						if _, exists := cmdFlags[flag.Name]; !exists {
							if flag.Short != "" {
								if _, exists := cmdFlags[flag.Short]; !exists {
									return fmt.Errorf("required flag --%s or -%s not provided", flag.Name, flag.Short)
								}
							} else {
								return fmt.Errorf("required flag --%s not provided", flag.Name)
							}
						}
					}
				}
				
				return cmd.Run(ctx, cmdArgs, cmdFlags)
			}
		}
	}

	// Command not found
	fmt.Printf("Unknown command: %s\n", cmdName)
	printHelp(config)
	return fmt.Errorf("unknown command: %s", cmdName)
}

// parseArgs separates flags from positional arguments
func parseArgs(args []string, flags []Flag) ([]string, map[string]string) {
	cmdArgs := []string{}
	cmdFlags := make(map[string]string)
	
	i := 0
	for i < len(args) {
		arg := args[i]
		
		if strings.HasPrefix(arg, "--") {
			flagName := arg[2:]
			// Check if this is a known flag
			isKnownFlag := false
			for _, f := range flags {
				if f.Name == flagName || f.Short == flagName {
					isKnownFlag = true
					break
				}
			}
			
			if isKnownFlag {
				// Check if next argument is a value for this flag
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") && !strings.HasPrefix(args[i+1], "-") {
					cmdFlags[flagName] = args[i+1]
					i += 2 // skip both flag and value
				} else {
					// Boolean flag (no value)
					cmdFlags[flagName] = ""
					i++
				}
			} else {
				// Unknown flag, treat as positional arg
				cmdArgs = append(cmdArgs, arg)
				i++
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			// Short flag
			flagName := arg[1:]
			// Check if this is a known flag
			isKnownFlag := false
			for _, f := range flags {
				if f.Short == flagName || f.Name == flagName {
					isKnownFlag = true
					break
				}
			}
			
			if isKnownFlag {
				// Check if next argument is a value for this flag
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") && !strings.HasPrefix(args[i+1], "-") {
					cmdFlags[flagName] = args[i+1]
					i += 2 // skip both flag and value
				} else {
					// Boolean flag (no value)
					cmdFlags[flagName] = ""
					i++
				}
			} else {
				// Unknown flag, treat as positional arg
				cmdArgs = append(cmdArgs, arg)
				i++
			}
		} else {
			// Positional argument
			cmdArgs = append(cmdArgs, arg)
			i++
		}
	}
	
	return cmdArgs, cmdFlags
}

// printHelp prints the help message for the application
func printHelp(config AppConfig) {
	fmt.Printf("%s - %s\n\n", config.Name, config.Description)
	fmt.Printf("Version: %s\n\n", config.Version)
	
	if len(config.Commands) > 0 {
		fmt.Println("Available commands:")
		for _, cmd := range config.Commands {
			fmt.Printf("  %s - %s\n", cmd.Name, cmd.Short)
			
			// Print flags for this command if it doesn't have subcommands
			if len(cmd.SubCommands) == 0 && len(cmd.Flags) > 0 {
				fmt.Printf("    Flags:\n")
				for _, flag := range cmd.Flags {
					if flag.Short != "" {
						fmt.Printf("      --%s, -%s: %s\n", flag.Name, flag.Short, flag.Usage)
					} else {
						fmt.Printf("      --%s: %s\n", flag.Name, flag.Usage)
					}
				}
			}
			
			// Print subcommands if they exist
			if len(cmd.SubCommands) > 0 {
				fmt.Printf("    Subcommands:\n")
				for _, subCmd := range cmd.SubCommands {
					fmt.Printf("      %s - %s\n", subCmd.Name, subCmd.Short)
					
					// Print flags for subcommand
					if len(subCmd.Flags) > 0 {
						fmt.Printf("        Flags:\n")
						for _, flag := range subCmd.Flags {
							if flag.Short != "" {
								fmt.Printf("          --%s, -%s: %s\n", flag.Name, flag.Short, flag.Usage)
							} else {
								fmt.Printf("          --%s: %s\n", flag.Name, flag.Usage)
							}
						}
					}
				}
			}
		}
		fmt.Println("\nUse --help for more information about a command.")
	}
}

// printCommandHelp prints help for a specific command
func printCommandHelp(cmd Command) {
	fmt.Printf("%s - %s\n", cmd.Name, cmd.Short)
	if cmd.Long != "" {
		fmt.Printf("\n%s\n", cmd.Long)
	}
	
	// Print flags for this command if it doesn't have subcommands
	if len(cmd.SubCommands) == 0 && len(cmd.Flags) > 0 {
		fmt.Printf("\nFlags:\n")
		for _, flag := range cmd.Flags {
			if flag.Short != "" {
				fmt.Printf("  --%s, -%s: %s\n", flag.Name, flag.Short, flag.Usage)
			} else {
				fmt.Printf("  --%s: %s\n", flag.Name, flag.Usage)
			}
		}
	}
	
	// Print subcommands if they exist
	if len(cmd.SubCommands) > 0 {
		fmt.Printf("\nSubcommands:\n")
		for _, subCmd := range cmd.SubCommands {
			fmt.Printf("  %s - %s\n", subCmd.Name, subCmd.Short)
			
			// Print flags for subcommand
			if len(subCmd.Flags) > 0 {
				fmt.Printf("    Flags:\n")
				for _, flag := range subCmd.Flags {
					if flag.Short != "" {
						fmt.Printf("      --%s, -%s: %s\n", flag.Name, flag.Short, flag.Usage)
					} else {
						fmt.Printf("      --%s: %s\n", flag.Name, flag.Usage)
					}
				}
			}
		}
	}
}

// Execute runs the app with os.Args
func Execute(config AppConfig) {
	if err := Run(config, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}