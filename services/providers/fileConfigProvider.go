package providers

import "sync"

const fileConfigPath = "./configs/files.yaml"

var (
	fileConfigOnce sync.Once
	fileConfigData *FileConfig
)

type FileOptions struct {
	FileFolder  *string `yaml:"file_folder,omitempty"`
	MaxFileSize *int    `yaml:"max_file_size,omitempty"`
}

type FileConfig struct {
	FileFolder  *string                 `yaml:"file_folder"`
	MaxFileSize *int                    `yaml:"max_file_size"`
	Files       map[string]*FileOptions `yaml:"files"`
}

func GetFileConfig() *FileConfig {
	fileConfigOnce.Do(func() {
		cfg := &FileConfig{}
		err := loadConfig(fileConfigPath, cfg)
		if err != nil {
			panic(err)
		}
		fileConfigData = cfg
	})
	return fileConfigData
}

func GetConfigByFileType(filetype string) FileOptions {
	cfg := GetFileConfig()
	cfgByFile := FileOptions{}
	for name, opt := range cfg.Files {
		if name == filetype {
			cfgByFile = *opt
		}
	}

	return FileOptions{
		FileFolder:  getValueOrDefault(cfgByFile.FileFolder, cfg.FileFolder),
		MaxFileSize: getValueOrDefault(cfgByFile.MaxFileSize, cfg.MaxFileSize),
	}
}

func getValueOrDefault[T any](value *T, defaultValue *T) *T {
	if value != nil {
		return value
	}
	return defaultValue
}
