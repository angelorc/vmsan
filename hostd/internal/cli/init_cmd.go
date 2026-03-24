package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a vmsan.toml configuration file",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().Bool("yes", false, "Non-interactive mode with auto-detected defaults")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	yes, _ := cmd.Flags().GetBool("yes")

	// Check if vmsan.toml already exists
	if _, err := os.Stat("vmsan.toml"); err == nil {
		return fmt.Errorf("vmsan.toml already exists in current directory")
	}

	// Auto-detect project
	detected := config.DetectProject(".")

	if yes {
		return initNonInteractive(detected)
	}
	return initInteractive(detected)
}

func initNonInteractive(detected *config.DetectionResult) error {
	runtime := "base"
	build := ""
	start := ""

	if detected != nil {
		runtime = detected.Runtime
		build = detected.Build
		start = detected.Start
	}

	if start == "" {
		return fmt.Errorf("could not auto-detect start command; run vmsan init without --yes for interactive mode")
	}

	return writeToml(runtime, build, start, "", "", "")
}

func initInteractive(detected *config.DetectionResult) error {
	reader := bufio.NewReader(os.Stdin)

	// Detect defaults
	defaultRuntime := "base"
	defaultBuild := ""
	defaultStart := ""
	if detected != nil {
		defaultRuntime = detected.Runtime
		defaultBuild = detected.Build
		defaultStart = detected.Start
		fmt.Printf("  Detected: %s (%s confidence)\n\n", detected.Reason, detected.Confidence)
	}

	runtime := prompt(reader, "Runtime", defaultRuntime)
	if !isKnownRuntime(runtime) {
		fmt.Printf("  Warning: Unknown runtime %q. Valid runtimes: %s\n", runtime, strings.Join(config.ValidRuntimes, ", "))
	}

	build := prompt(reader, "Build command", defaultBuild)
	start := prompt(reader, "Start command", defaultStart)

	// Optional database
	dbChoice := prompt(reader, "Database (postgres/mysql/none)", "none")
	db := ""
	if dbChoice != "none" && dbChoice != "" {
		db = dbChoice
	}

	// Optional redis
	redisChoice := prompt(reader, "Redis (yes/no)", "no")
	redis := ""
	if strings.ToLower(redisChoice) == "yes" || strings.ToLower(redisChoice) == "y" {
		redis = "redis"
	}

	// Project name
	cwd, _ := os.Getwd()
	defaultProject := strings.ToLower(sanitizeName(lastPathSegment(cwd)))
	project := prompt(reader, "Project name", defaultProject)

	return writeToml(runtime, build, start, db, redis, project)
}

func writeToml(runtime, build, start, db, redis, project string) error {
	var sb strings.Builder

	if project != "" {
		sb.WriteString(fmt.Sprintf("project = %q\n\n", project))
	}

	sb.WriteString("[services.web]\n")
	if runtime != "" && runtime != "base" {
		sb.WriteString(fmt.Sprintf("runtime = %q\n", runtime))
	}
	if build != "" {
		sb.WriteString(fmt.Sprintf("build = %q\n", build))
	}
	if start != "" {
		sb.WriteString(fmt.Sprintf("start = %q\n", start))
	}

	// Dependencies
	var deps []string
	if db != "" {
		deps = append(deps, "db")
	}
	if redis != "" {
		deps = append(deps, "cache")
	}

	if len(deps) > 0 {
		quoted := make([]string, len(deps))
		for i, d := range deps {
			quoted[i] = fmt.Sprintf("%q", d)
		}
		sb.WriteString(fmt.Sprintf("depends_on = [%s]\n", strings.Join(quoted, ", ")))

		// connect_to for database access
		var connectTo []string
		if db != "" {
			port := "5432"
			if db == "mysql" {
				port = "3306"
			}
			connectTo = append(connectTo, fmt.Sprintf("db:%s", port))
		}
		if redis != "" {
			connectTo = append(connectTo, "cache:6379")
		}
		if len(connectTo) > 0 {
			quotedCT := make([]string, len(connectTo))
			for i, ct := range connectTo {
				quotedCT[i] = fmt.Sprintf("%q", ct)
			}
			sb.WriteString(fmt.Sprintf("connect_to = [%s]\n", strings.Join(quotedCT, ", ")))
		}
	}

	// Accessories
	if db != "" {
		sb.WriteString(fmt.Sprintf("\n[accessories.db]\ntype = %q\n", db))
	}
	if redis != "" {
		sb.WriteString("\n[accessories.cache]\ntype = \"redis\"\n")
	}

	content := sb.String()
	fmt.Println()
	fmt.Println(content)

	if err := os.WriteFile("vmsan.toml", []byte(content), 0644); err != nil {
		return fmt.Errorf("write vmsan.toml: %w", err)
	}

	fmt.Println("  Created vmsan.toml")
	return nil
}

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func isKnownRuntime(r string) bool {
	for _, v := range config.ValidRuntimes {
		if v == r {
			return true
		}
	}
	return false
}

func sanitizeName(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			result.WriteRune(r - 'A' + 'a')
		} else if r == '_' || r == ' ' {
			result.WriteRune('-')
		}
	}
	return result.String()
}

func lastPathSegment(path string) string {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return "app"
}
