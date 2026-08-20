package log

// defaultConfig 是构建默认 logger 的兜底配置：单个 console 输出，debug 级别，text 格式。
var defaultConfig = Config{
	{
		Writer:       "console",
		Level:        "debug",
		Format:       "text",
		WriterConfig: WriterConfig{},
	},
}

// Config 是日志配置：一组相互独立的输出目标。
type Config []OutputConfig

// OutputConfig 描述单个输出目标：写入位置（Writer）、最低级别、格式，以及 writer 相关配置。
type OutputConfig struct {
	Writer       string       `yaml:"writer"`
	Level        string       `yaml:"level"`
	Format       string       `yaml:"format"`
	WriterConfig WriterConfig `yaml:"writer_config"`
}

// WriterConfig 配置文件 writer（仅当 Writer == "file" 时生效）。MaxSize 单位为 MB，MaxAge 单位为天，Compress 对轮转文件做 gzip 压缩。
type WriterConfig struct {
	LogPath    string `yaml:"log_path"`
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
	Compress   bool   `yaml:"compress"`
}
