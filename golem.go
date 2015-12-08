package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/dmcgowan/golem/buildutil"
	"github.com/dmcgowan/golem/clientutil"
	"github.com/dmcgowan/golem/versionutil"
)

func main() {
	name := filepath.Base(os.Args[0])
	if name == "golem_runner" {
		runnerMain()
		return
	}
	var (
		dockerBinary string
		cacheDir     string
		buildCache   string
	)
	co := clientutil.NewClientOptions()
	cm := NewConfigurationManager()

	// Move Docker Specific options to separate type
	flag.StringVar(&dockerBinary, "db", "", "Docker binary to test")
	flag.StringVar(&cacheDir, "cache", "", "Cache directory")
	flag.StringVar(&buildCache, "build-cache", "", "Build cache location, if outside of default cache directory")
	// TODO: Add swarm flag and host option

	flag.Parse()

	// TODO: Allow quiet and verbose mode
	logrus.SetLevel(logrus.DebugLevel)

	if cacheDir == "" {
		td, err := ioutil.TempDir("", "build-cache-")
		if err != nil {
			logrus.Fatalf("Error creating tempdir: %v", err)
		}
		cacheDir = td
		defer os.RemoveAll(td)
	}

	if buildCache == "" {
		buildCache = filepath.Join(cacheDir, "builds")
		if err := os.MkdirAll(buildCache, 0755); err != nil {
			logrus.Fatalf("Error creating build cache directory")
		}
	}
	c := CacheConfiguration{
		ImageCache: &ImageCache{
			root: filepath.Join(cacheDir, "images"),
		},
		BuildCache: buildutil.NewFSBuildCache(buildCache),
	}

	if dockerBinary != "" {
		v, err := versionutil.BinaryVersion(dockerBinary)
		if err != nil {
			logrus.Fatalf("Error getting binary version of %s: %v", dockerBinary, err)
		}
		logrus.Debugf("Using local binary with version %s", v.String())
		if err := c.BuildCache.PutVersion(v, dockerBinary); err != nil {
			logrus.Fatalf("Error putting %s in cache as %s: %v", dockerBinary, v, err)
		}

		flag.Set("docker-version", v.String())
	}

	client, err := NewDockerClient(co)
	if err != nil {
		logrus.Fatalf("Failed to create client: %v", err)
	}

	v, err := client.Version()
	if err != nil {
		logrus.Fatalf("Error getting version: %v", err)
	}

	serverVersion, err := versionutil.ParseVersion(v.Get("Version"))
	if err != nil {
		logrus.Fatalf("Unexpected version value: %s", v.Get("Version"))
	}
	// TODO: Support arbitrary load version instead of server version by
	// starting up separate daemon for load
	// TODO: Check cache here to ensure that load will not have issues
	logrus.Debugf("Using docker daemon for image export, version %s", serverVersion)

	runnerConfig, err := cm.RunnerConfiguration(serverVersion)
	if err != nil {
		logrus.Fatalf("Error creating runner configuration: %v", err)
	}
	runner := NewRunner(runnerConfig, c)

	if err := runner.Build(client); err != nil {
		logrus.Fatalf("Error building test images: %v", err)
	}

	if err := runner.Run(client); err != nil {
		logrus.Fatalf("Error running tests: %v", err)
	}
}

func runnerMain() {
	// TODO: Parse runner options
	flag.Parse()

	// TODO: Allow quiet and verbose mode
	logrus.SetLevel(logrus.DebugLevel)

	logrus.Debugf("Runner!")

	scriptCapturer := newFileCapturer("scripts")
	defer scriptCapturer.Close()
	loadCapturer := newFileCapturer("load")
	defer loadCapturer.Close()
	daemonCapturer := newFileCapturer("daemon")
	defer daemonCapturer.Close()
	testCapturer := NewConsoleLogCapturer()
	defer testCapturer.Close()
	composeCapturer := newFileCapturer("compose")
	defer composeCapturer.Close()

	suiteConfig := SuiteRunnerConfiguration{
		DockerInDocker:        true,
		CleanDockerGraph:      false,
		DockerLoadLogCapturer: loadCapturer,
		DockerLogCapturer:     daemonCapturer,
		SetupScripts: [][]string{
			{"/bin/sh", "/runner/install_certs.sh", "localregistry"},
		},
		SetupLogCapturer: scriptCapturer,
		ComposeFile:      "/runner/docker-compose.yml",
		ComposeCapturer:  composeCapturer,

		TestCapturer: testCapturer,
		TestCommand:  "bats",
		TestArgs:     []string{"-t", "registry"},
		TestEnv: []string{
			"TEST_REPO=hello-world",
			"TEST_TAG=latest",
			"TEST_USER=testuser",
			"TEST_PASSWORD=passpassword",
			"TEST_REGISTRY=localregistry",
			"TEST_SKIP_PULL=true",
		},
	}

	runner := NewSuiteRunner(suiteConfig)

	if err := runner.Setup(); err != nil {
		logrus.Fatalf("Setup error: %v", err)
	}

	runErr := runner.RunTests()

	if err := runner.TearDown(); err != nil {
		logrus.Errorf("TearDown error: %v", err)
	}

	if runErr != nil {
		logrus.Fatalf("Test errored: %v", runErr)
	}

}

func newFileCapturer(name string) LogCapturer {
	basename := filepath.Join("/var/log/docker", name)
	lc, err := NewFileLogCapturer(basename)
	if err != nil {
		logrus.Fatalf("Error creating file capturer for %s: %v", basename, err)
	}

	return lc
}