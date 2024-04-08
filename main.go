package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const (
	roleCode = "Provide only code as output without any description." +
		"Provide only code in plain text format without Markdown formatting. Do not" +
		"include symbols such as ``` or ```python. If there is a lack of details," +
		"provide most logical solution. You are not allowed to ask for more details." +
		"For example if the prompt is \"Hello world Python\", you should return" +
		"\"print('Hello world')\"."

	roleShell = "Provide only %s commands for %s without any description." +
		"If there is a lack of details, provide most logical solution." +
		"Ensure the output is a valid shell command." +
		"If multiple steps required try to combine them together using &&." +
		"Provide only plain text without Markdown formatting." +
		"Do not provide markdown formatting such as ```."

	roleDescribeShell = "Provide a terse, single sentence description of the given shell command." +
		"Describe each argument and option of the command." +
		"Provide short responses in about 80 words." +
		"APPLY MARKDOWN formatting when possible."

	roleDefault = "You are programming and system administration assistant." +
		"You are managing %s operating system with %s shell." +
		"Provide short responses in about 100 words, unless you are specifically asked for more details." +
		"If you need to store any data, assume it will be stored in the conversation." +
		"APPLY MARKDOWN formatting when possible."

	ollamaChatURL = "/api/chat"
)

var (
	shellMsg     string
	codeMsg      string
	descMsg      string
	verbose      bool
	showConfig   bool
	createConfig bool
	versionFlag  bool
	version      string = "dev"
	commitSHA    string = "none"
	buildDate    string = "unknown"
)

type Config struct {
	OLLAMA_BASE_URL string `yaml:"OLLAMA_BASE_URL"`
	OLLAMA_MODEL    string `yaml:"OLLAMA_MODEL"`
}

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type APIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type APIResponse struct {
	Model    string   `json:"model"`
	Response string   `json:"response"`
	Message  *Message `json:"message"`
}

func loadConfig(createIfNotExists bool) (Config, error) {
	var config Config

	// Set default values
	config.OLLAMA_MODEL = "openhermes2.5-mistral"
	config.OLLAMA_BASE_URL = "http://localhost:11434"

	usr, err := user.Current()
	if err != nil {
		return config, fmt.Errorf("failed to get user: %v", err)
	}

	configFilePath := filepath.Join(usr.HomeDir, ".config", "gaia", "config.yaml")

	//yamlFile, err := ioutil.ReadFile(configFilePath)
	yamlFile, err := os.ReadFile(configFilePath)
	if err != nil {
		if createIfNotExists {
			// Config file doesn't exist, create it with default values
			if err := createDefaultConfig(configFilePath, config); err != nil {
				return config, fmt.Errorf("failed to create config file: %v", err)
			}
		} else {
			// Config file doesn't exist, return default values
			return config, nil
		}
	}

	// Load config from file
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return config, fmt.Errorf("failed to unmarshal YAML: %v", err)
	}

	return config, nil
}

func createDefaultConfig(filePath string, config Config) error {
	// Create config directory if it doesn't exist
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		return err
	}

	// Marshal default config to YAML
	defaultConfig, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	// Write default config to file
	//err = ioutil.WriteFile(filePath, defaultConfig, 0644)
	err = os.WriteFile(filePath, defaultConfig, 0644)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "app [options] [message]",
		Short: "gaia is a CLI tool",
		Run: func(cmd *cobra.Command, args []string) {
			if versionFlag {
				fmt.Printf("Gaia %s, commit %s, built at %s\n", version, commitSHA, buildDate)
				os.Exit(0)
			}

			config, err := loadConfig(createConfig)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			if showConfig || createConfig {
				displayConfig()
				os.Exit(0)
			}

			msg := ""
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				buf := make([]byte, 4096)
				n, _ := os.Stdin.Read(buf)
				msg = string(buf[:n])
			}

			var role string
			message := strings.TrimSpace(msg)

			if shellMsg != "" {
				message += " " + shellMsg
				role = fmt.Sprintf(roleShell, os.Getenv("SHELL"), runtime.GOOS)
			} else if codeMsg != "" {
				message += " " + codeMsg
				role = roleCode
			} else if descMsg != "" {
				message += " " + descMsg
				role = roleDescribeShell
			} else {
				message += strings.Join(args, " ")
				role = fmt.Sprintf(roleDefault, runtime.GOOS, os.Getenv("SHELL"))
			}

			if message == "" || strings.TrimSpace(message) == "" {
				if err := cmd.Usage(); err != nil {
					fmt.Printf("error displaying usage: %v\n", err)
				}
				return
			}

			if verbose {
				fmt.Println(message)
				fmt.Println(config)
				fmt.Println(role)
			}

			processMessage(message, config, role)
		},
	}

	rootCmd.Flags().StringVarP(&shellMsg, "shell", "s", "", "message for shell option")
	rootCmd.Flags().StringVarP(&codeMsg, "code", "c", "", "message for code option")
	rootCmd.Flags().StringVarP(&descMsg, "description", "d", "", "message for description option")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVarP(&showConfig, "show-config", "g", false, "display current config")
	rootCmd.Flags().BoolVarP(&createConfig, "create-config", "t", false, "create config file if it doesn't exist")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "V", false, "version")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func processMessage(msg string, config Config, role string) {
	err := callAPI(config, "POST", "application/json", msg, role)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func callAPI(config Config, method, contentType string, body string, role string) error {
	request := APIRequest{
		Model: config.OLLAMA_MODEL,
		Messages: []Message{
			{
				Role:    "system",
				Content: role,
			},
			{
				Role:    "user",
				Content: body,
			},
		},
		Stream: true,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		fmt.Println("Error during call on API")
		return fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	resp, err := http.Post(config.OLLAMA_BASE_URL+ollamaChatURL, contentType, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %v", err)
	}
	defer resp.Body.Close()

	processStreamedResponse(resp.Body)
	return nil
}

func displayConfig() {
	config, err := loadConfig(false)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	fmt.Println("Current Config:")
	fmt.Println("OLLAMA_MODEL:", config.OLLAMA_MODEL)
	fmt.Println("OLLAMA_BASE_URL:", config.OLLAMA_BASE_URL)
}

func processStreamedResponse(body io.Reader) {
	decoder := json.NewDecoder(body)

	fmt.Println()

	respChan := make(chan string)
	eofChan := make(chan struct{})
	errorChan := make(chan error)
	go func() {
		var apiResp APIResponse
		for {
			if err := decoder.Decode(&apiResp); err == io.EOF {
				eofChan <- struct{}{}
			} else if err != nil {
				errorChan <- err
			} else {
				respChan <- apiResp.Message.Content
			}
		}
	}()

	for {
		select {
		case resp := <-respChan:
			fmt.Print(resp)
		case err := <-errorChan:
			fmt.Println("Error decoding JSON:", err)
			return
		case <-eofChan:
			fmt.Println()
			return
		}
	}
}
