# Go Application Template

I end up creating the same boilerplate over and over, so I created this template for Go applications using Cobra CLI framework. This template provides a solid foundation with best practices, tooling, and development container support.

## Features

- 🚀 **Cobra CLI Framework** - Structured command-line interface
- 📝 **Zap Logging** - High-performance structured logging
- 🔍 **Linting** - golangci-lint with comprehensive rules
- 🔐 **Security Scanning** - Trivy for vulnerability detection
- 🐳 **DevContainer Ready** - Easy development environment setup
- 🛠️ **Makefile** - Common development tasks
- ✅ **Testing Support** - Unit test framework with coverage

## Quick Start

### Prerequisites

- Go 1.25.0 or later
- Make
- Docker (for devcontainer or containerized builds)

### Local Development

1. **Clone and customize the template:**
   ```bash
   git clone <your-repo-url>
   cd <your-project>
   ```

2. **Update the module name:**
   ```bash
   # Update go.mod with your module path
   go mod edit -module github.com/your-org/your-project
   ```

3. **Install dependencies:**
   ```bash
   make vendor
   ```

4. **Run the application:**
   ```bash
   make serve
   # or
   go run main.go serve
   ```

### Development Container (Recommended)

The easiest way to get started is using VS Code's DevContainer feature. This ensures a consistent development environment for all team members.

#### Option 1: Generate DevContainer using VS Code

1. **Open the project in VS Code**
2. **Press `F1` or `Cmd/Ctrl + Shift + P`**
3. **Select "Dev Containers: Add Dev Container Configuration Files..."**
4. **Choose "Go" from the list of container definitions**
5. **Select the appropriate Go version (1.25.0 or later)**
6. **VS Code will generate `.devcontainer/devcontainer.json`**

#### Option 2: Create DevContainer Manually

Create a `.devcontainer/devcontainer.json` file:

```json
{
  "name": "Go Development",
  "image": "golang:1.25",
  "features": {
    "ghcr.io/devcontainers/features/github-cli:1": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "customizations": {
    "vscode": {
      "extensions": [
        "golang.go",
        "ms-vscode.makefile-tools"
      ],
      "settings": {
        "go.toolsManagement.checkForUpdates": "local",
        "go.useLanguageServer": true
      }
    }
  },
  "postCreateCommand": "go mod download",
  "remoteUser": "vscode"
}
```

#### Option 3: Use Docker Compose (for services)

If your application needs additional services (databases, etc.), create `.devcontainer/docker-compose.yml`:

```yaml
version: '3.8'

services:
  app:
    build:
      context: ..
      dockerfile: .devcontainer/Dockerfile
    volumes:
      - ..:/workspace:cached
    command: sleep infinity
    network_mode: service:db

  db:
    image: postgres:latest
    restart: unless-stopped
    volumes:
      - postgres-data:/var/lib/postgresql/data
    environment:
      POSTGRES_PASSWORD: postgres
      POSTGRES_USER: postgres
      POSTGRES_DB: appdb

volumes:
  postgres-data:
```

Then update `.devcontainer/devcontainer.json` to use the compose file:

```json
{
  "name": "Go Development",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  }
}
```

#### Opening the DevContainer

1. **Open the project in VS Code**
2. **Press `F1` or `Cmd/Ctrl + Shift + P`**
3. **Select "Dev Containers: Reopen in Container"**
4. VS Code will build and start the container
5. Wait for the container to initialize (first time may take a few minutes)

## Project Structure

```
.
├── cmd/              # Command implementations
│   ├── root.go      # Root command and CLI setup
│   └── serve.go     # Serve command implementation
├── main.go          # Application entry point
├── go.mod           # Go module dependencies
├── Makefile         # Common development tasks
├── .golangci.yaml   # Linter configuration
├── .trivy.yaml      # Security scanner configuration
└── .devcontainer/   # DevContainer configuration (create this)
    └── devcontainer.json
```

## Available Make Targets

```bash
make help          # Show available targets (if defined)
make vendor        # Download and tidy dependencies
make serve         # Run the application in development mode
make test          # Run all tests (lint + unit tests + security checks)
make unit-test     # Run unit tests with coverage
make lint          # Run golangci-lint
make vulncheck     # Run vulnerability checks (govulncheck + trivy)
make build         # Build the application and Docker image
make clean         # Clean build artifacts and test cache
```

## Customization Guide

### 1. Update Application Name

Replace `CHANGEME` throughout the codebase:

- `cmd/root.go`: Update `appName` and `appDescription` constants
- `Makefile`: Update `CHANGEME` in build targets
- `go.mod`: Update module path

### 2. Add Your Commands

Create new command files in `cmd/` directory:

```go
// cmd/yourcommand.go
package cmd

var yourCmd = &cobra.Command{
    Use:   "yourcommand",
    Short: "Description of your command",
    Run: func(cmd *cobra.Command, args []string) {
        // Your command logic
    },
}

func init() {
    rootCmd.AddCommand(yourCmd)
}
```

### 3. Configure Logging

The template uses Zap for logging. Configure it in `cmd/root.go`:

```go
// Uncomment and customize configureLogger function
logger = configureLogger(context.Background(), cfg)
```

### 4. Add Configuration

Create a configuration package and uncomment configuration loading in `cmd/root.go`:

```go
// Load configuration
cfg = YourApp.NewDefaultConfig()
if err := YourApp.Load(configFile, configEnv); err != nil {
    panic(err)
}
```

## Development Workflow

1. **Create a feature branch:**
   ```bash
   git checkout -b feature/your-feature
   ```

2. **Make your changes** and run tests:
   ```bash
   make test
   ```

3. **Fix linting issues:**
   ```bash
   make lint
   ```

4. **Ensure security checks pass:**
   ```bash
   make vulncheck
   ```

5. **Commit and push:**
   ```bash
   git commit -m "feat: your feature description"
   git push origin feature/your-feature
   ```

## Tools and Dependencies

### Required Tools

- **golangci-lint**: Code linting
  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```

- **govulncheck**: Go vulnerability checker
  ```bash
  go install golang.org/x/vuln/cmd/govulncheck@latest
  ```

- **trivy**: Security scanner
  ```bash
  # Install via package manager or download from: https://github.com/aquasecurity/trivy
  ```

### Key Dependencies

- `github.com/spf13/cobra` - CLI framework
- `go.uber.org/zap` - Structured logging

## Troubleshooting

### DevContainer Issues

- **Container won't start**: Check that Docker is running and you have sufficient resources allocated
- **Extensions not loading**: Ensure the container has finished building completely
- **Go tools not found**: Run `go mod download` in the container terminal

### Build Issues

- **Module not found**: Run `make vendor` to download dependencies
- **Lint errors**: Run `make lint` to see specific issues, then fix them
- **Test failures**: Run `make unit-test` to see detailed test output

## Contributing

1. Fork the repository
2. Create your feature branch
3. Make your changes
4. Run `make test` to ensure everything passes
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Support

[Add support information or links here]

