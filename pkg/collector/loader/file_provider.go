package loader

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"

	"gopkg.in/yaml.v2"
)

type configFormat struct {
	InitConfig interface{} `yaml:"init_config"`
	Instances  []check.ConfigRawMap
}

// FileConfigProvider collect configuration files from disk
type FileConfigProvider struct {
	paths []string
}

// NewFileConfigProvider creates a new FileConfigProvider searching for
// configuration files on the given paths
func NewFileConfigProvider(paths []string) *FileConfigProvider {
	return &FileConfigProvider{paths: paths}
}

// Collect scans provided paths searching for configuration files. When found,
// it parses the files and try to unmarshall Yaml contents into a CheckConfig
// instance
func (c *FileConfigProvider) Collect() ([]check.Config, error) {
	configs := []check.Config{}

	for _, path := range c.paths {
		log.Infof("Searching for configuration files at: %s", path)

		entries, err := ioutil.ReadDir(path)
		if err != nil {
			log.Warnf("Skipping, %s", err)
			continue
		}

		for _, entry := range entries {
			ext := filepath.Ext(entry.Name())

			// skip config files of type check.yaml.example
			if ext == ".example" {
				log.Debugf("Skipping file: %s", entry.Name())
				continue
			}

			if entry.IsDir() {
				configs = append(configs, collectDir(path, entry)...)
			} else {
				checkName := entry.Name()[:len(entry.Name())-len(ext)]
				conf, err := getCheckConfig(checkName, filepath.Join(path, entry.Name()))
				if err != nil {
					log.Warnf("%s is not a valid config file: %s", entry.Name(), err)
					continue
				}
				log.Debug("Found valid configuration in file:", entry.Name())
				configs = append(configs, conf)
			}
		}
	}

	return configs, nil
}

func collectDir(parentPath string, folder os.FileInfo) []check.Config {
	configs := []check.Config{}

	if filepath.Ext(folder.Name()) != ".d" {
		// the name of this directory isn't in the form `checkname.d`, skip it
		log.Debugf("Not a config folder, skipping directory: %s", folder.Name())
		return configs
	}

	dirPath := filepath.Join(parentPath, folder.Name())

	// search for yaml files within this directory
	subEntries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Warnf("Skipping config directory: %s", err)
		return configs
	}

	// strip the trailing `.d`
	checkName := folder.Name()[:len(folder.Name())-2]

	// try to load any config file in it
	for _, sEntry := range subEntries {
		if !sEntry.IsDir() {
			filePath := filepath.Join(dirPath, sEntry.Name())
			conf, err := getCheckConfig(checkName, filePath)
			if err != nil {
				log.Warnf("%s is not a valid config file: %s", sEntry.Name(), err)
				continue
			}
			log.Debug("Found valid configuration in file:", filePath)
			configs = append(configs, conf)
		}
	}

	return configs
}

// getCheckConfig returns an instance of check.Config if `fpath` points to a valid config file
func getCheckConfig(name, fpath string) (check.Config, error) {
	cf := configFormat{}
	config := check.Config{Name: name}

	// Read file contents
	// FIXME: ReadFile reads the entire file, possible security implications
	yamlFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return config, err
	}

	// Parse configuration
	err = yaml.Unmarshal(yamlFile, &cf)
	if err != nil {
		return config, err
	}

	// If no valid instances were found, this is not a valid configuration file
	if len(cf.Instances) < 1 {
		return config, errors.New("Configuration file contains no valid instances")
	}

	// at this point the Yaml was already parsed, no need to check the error
	rawInitConfig, _ := yaml.Marshal(cf.InitConfig)
	config.InitConfig = rawInitConfig

	// Go through instances and return corresponding []byte
	for _, instance := range cf.Instances {
		// at this point the Yaml was already parsed, no need to check the error
		rawConf, _ := yaml.Marshal(instance)
		config.Instances = append(config.Instances, rawConf)
	}

	return config, err
}
