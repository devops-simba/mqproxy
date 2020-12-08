package main

import (
	"os"
	"path/filepath"

	"github.com/devops-simba/helpers"
)

var logFactory helpers.LogFactory
var mainLogger helpers.Logger

type LoggingConfig struct {
	Level     *helpers.LogLevelUnmarshaller `yaml:"level"`
	Verbosity *int                          `yaml:"verbosity"`
	Output    string                        `yaml:"output,omitempty"`
	Template  string                        `yaml:"template,omitempty"`
}

func GetLogFactory() helpers.LogFactory { return logFactory }
func GetMainLogger() helpers.Logger {
	if mainLogger == nil {
		panic("Logging is not inititialized yet")
	}
	return mainLogger
}
func InitializeLogging(config *LoggingConfig) error {
	defaultVerbosity := 0
	defaultLevel := helpers.LogLevelUnmarshaller{Level: helpers.Info}
	defaultLogTemplate := `{{ .Level.Format "l" }}{{ .LogTime.Format "01/02-03:04:05.999" }} {{ .LogSource }} {{ .Content | WithColorC $ "" }}`
	if config == nil {
		config = &LoggingConfig{
			Level:     &defaultLevel,
			Verbosity: &defaultVerbosity,
			Output:    "",
			Template:  defaultLogTemplate,
		}
	} else {
		if config.Level == nil {
			config.Level = &defaultLevel
		}
		if config.Verbosity == nil {
			config.Verbosity = &defaultVerbosity
		}
		if config.Template == "" {
			config.Template = defaultLogTemplate
		}
	}

	format, err := helpers.ParseTemplate("LogFormat", config.Template)
	if err != nil {
		return err
	}

	var output *os.File
	mustCloseOutput := false
	switch config.Output {
	case "", "-", "stdout":
		output = os.Stdout
	case "2", "stderr":
		output = os.Stderr
	default:
		dirName := filepath.Dir(config.Output)
		if _, err := os.Stat(dirName); err != nil && os.IsNotExist(err) {
			os.MkdirAll(dirName, 0777)
		}
		if output, err = os.OpenFile(config.Output, os.O_RDWR|os.O_APPEND|os.O_CREATE, os.ModeAppend); err != nil {
			return err
		}
		mustCloseOutput = true
	}
	if config.Output != "" && config.Output != "-" {

	}

	logFactory = helpers.NewFileLogFactory(format, output, config.Level.Level, *config.Verbosity, mustCloseOutput)
	mainLogger = logFactory.CreateLogger("main", nil, nil)
	return nil
}
func StopLogging() error {
	if logFactory == nil {
		panic("Logging is not initialized")
	}

	return logFactory.Close()
}

func CreateLogger(source string) helpers.Logger {
	return logFactory.CreateLogger(source, nil, nil)
}
