package sca

type SCAConfig struct {
	EnableDocker     bool
	DockerEndpoint   string
	DockerNumWorkers int

	/*
		include inspect by Image Names n Image LocalFiles
	*/
	DockerImages               []string
	DockerImageLocalFile       []string
	DockerSaveImageDirectories string // default to use os.CreateTemp

	/*
		Source Code / Repository FS Open Mount INTO CONTAINERS
		Try to Analyze the Source Code / Repository:

		1. Use Docker API Inspect Container Mounts Config
		2. Use FS Analyzer to make it
		3. Build Deps / SBOM
	*/
	DockerContainers []string

	FileSystemPath   string
	DisableLanguages []string
}

type SCAConfigOption func(*SCAConfig)

func WithDockerEndpoint(endpoint string) SCAConfigOption {
	return func(c *SCAConfig) {
		c.EnableDocker = true
		c.DockerEndpoint = endpoint
	}
}

func WithDocker(b bool) SCAConfigOption {
	return func(config *SCAConfig) {
		config.EnableDocker = b
	}
}

func WithDockerNumWorkers(n int) SCAConfigOption {
	return func(config *SCAConfig) {
		config.DockerNumWorkers = n
	}
}

func WithFileSystemPath(path string) SCAConfigOption {
	return func(config *SCAConfig) {
		config.FileSystemPath = path
	}
}

func WithDisableLanguages(languages ...string) SCAConfigOption {
	return func(config *SCAConfig) {
		config.DisableLanguages = languages
	}
}

func WithDockerImages(images ...string) SCAConfigOption {
	return func(config *SCAConfig) {
		config.DockerImages = images
	}
}
