package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

var CfgFile string

type Config struct{}

var config Config

func defaultConfig() *viper.Viper {
	v := viper.New()
	v.SetDefault("model", "mistral")
	v.SetDefault("host", "localhost")
	v.SetDefault("port", 11434)
	v.SetDefault("roles.default", "You are programming and system administration assistant. You are managing %s operating system with %s shell. Provide short responses in about 100 words, unless you are specifically asked for more details. If you need to store any data, assume it will be stored in the conversation. APPLY MARKDOWN formatting when possible.")
	v.SetDefault("roles.describe", "Provide a terse, single sentence description of the given shell command. Describe each argument and option of the command. Provide short responses in about 80 words. APPLY MARKDOWN formatting when possible.")
	v.SetDefault("roles.shell", "Provide only %s commands for %s without any description. If there is a lack of details, provide the most logical solution. Ensure the output is a valid shell command. If multiple steps are required, try to combine them using &&. Provide only plain text without Markdown formatting. Do not use markdown formatting such as ```.")
	v.SetDefault("roles.code", "Provide only code as output without any description. Provide only code in plain text format without Markdown formatting. Do not include symbols such as ``` or ```python. If there is a lack of details, provide most logical solution. You are not allowed to ask for more details. For example if the prompt is \"Hello world Python\", you should return \"print('Hello world')\".")

	return v
}

func InitConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		return err
	}

	configDir := filepath.Join(homeDir, ".config", "gaia")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		fmt.Println("Error creating config directory:", err)
		return err
	}

	CfgFile = filepath.Join(configDir, "config.yaml")

	viper.SetConfigFile(CfgFile)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	for key, value := range defaultConfig().AllSettings() {
		viper.SetDefault(key, value)
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Error reading config file:", err)
	}

	if err := viper.Unmarshal(&config); err != nil {
		fmt.Println("Error unmarshalling config:", err)
		return err
	}

	if err := viper.WriteConfig(); err != nil {
		fmt.Println("Error writing config file:", err)
	}

	return nil
}

func SetConfigString(key, value string) {
	if defaultConfig().IsSet(key) {
		viper.Set(key, value)
	} else {
		fmt.Println("No config found for key:", key, ":", value)
		os.Exit(1)
	}

	if err := viper.WriteConfig(); err != nil {
		fmt.Println("Error writing config file:", err)
	}
}
