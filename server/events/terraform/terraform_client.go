// Copyright 2017 HootSuite Media Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Modified hereafter by contributors to runatlantis/atlantis.
//
// Package terraform handles the actual running of terraform commands.
package terraform

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-version"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/logging"
)

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_terraform_client.go Client

type Client interface {
	// RunCommandWithVersion executes terraform with args in path. If v is nil,
	// it will use the default Terraform version. workspace is the Terraform
	// workspace which should be set as an environment variable.
	RunCommandWithVersion(ctx models.ProjectCommandContext, path string, args []string, envs map[string]string, v *version.Version) (string, error)

	// EnsureVersion makes sure that terraform version `v` is available to use
	EnsureVersion(log *logging.SimpleLogger, v *version.Version) error
}

type DefaultClient struct {
	// defaultVersion is the default version of terraform to use if another
	// version isn't specified.
	defaultVersion          *version.Version
	terraformPluginCacheDir string
	binDir                  string
	// overrideTF can be used to override the terraform binary during testing
	// with another binary, ex. echo.
	overrideTF string
	// downloader downloads terraform versions.
	downloader      Downloader
	downloadBaseURL string
	// versions maps from the string representation of a tf version (ex. 0.11.10)
	// to the absolute path of that binary on disk (if it exists).
	// Use versionsLock to control access.
	versions map[string]string

	// versionsLock is used to ensure versions isn't being concurrently written to.
	versionsLock *sync.Mutex

	// usePluginCache determines whether or not to set the TF_PLUGIN_CACHE_DIR env var
	usePluginCache bool

	terraformOutputChan chan<- *models.TerraformOutputLine
}

//go:generate pegomock generate -m --use-experimental-model-gen --package mocks -o mocks/mock_downloader.go Downloader

// Downloader is for downloading terraform versions.
type Downloader interface {
	GetFile(dst, src string, opts ...getter.ClientOption) error
}

const (
	// terraformPluginCacheDir is the name of the dir inside our data dir
	// where we tell terraform to cache plugins and modules.
	terraformPluginCacheDirName = "plugin-cache"
	// binDirName is the name of the directory inside our data dir where
	// we download terraform binaries.
	binDirName = "bin"
)

// versionRegex extracts the version from `terraform version` output.
//     Terraform v0.12.0-alpha4 (2c36829d3265661d8edbd5014de8090ea7e2a076)
//	   => 0.12.0-alpha4
//
//     Terraform v0.11.10
//	   => 0.11.10
var versionRegex = regexp.MustCompile("Terraform v(.*?)(\\s.*)?\n")

// NewClient constructs a terraform client.
// tfeToken is an optional terraform enterprise token.
// defaultVersionStr is an optional default terraform version to use unless
// a specific version is set.
// defaultVersionFlagName is the name of the flag that sets the default terraform
// version.
// tfDownloader is used to download terraform versions.
// Will asynchronously download the required version if it doesn't exist already.
func NewClient(
	log *logging.SimpleLogger,
	dataDir string,
	tfeToken string,
	tfeHostname string,
	defaultVersionStr string,
	defaultVersionFlagName string,
	tfDownloadURL string,
	tfDownloader Downloader,
	usePluginCache bool,
	terraformOutputChan chan<- *models.TerraformOutputLine,
) (*DefaultClient, error) {
	var finalDefaultVersion *version.Version
	var localVersion *version.Version
	versions := make(map[string]string)
	var versionsLock sync.Mutex

	localPath, err := exec.LookPath("terraform")
	if err != nil && defaultVersionStr == "" {
		return nil, fmt.Errorf("terraform not found in $PATH. Set --%s or download terraform from https://www.terraform.io/downloads.html", defaultVersionFlagName)
	}
	if err == nil {
		localVersion, err = getVersion(localPath)
		if err != nil {
			return nil, err
		}
		versions[localVersion.String()] = localPath
		if defaultVersionStr == "" {
			// If they haven't set a default version, then whatever they had
			// locally is now the default.
			finalDefaultVersion = localVersion
		}
	}

	binDir := filepath.Join(dataDir, binDirName)
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return nil, errors.Wrapf(err, "unable to create terraform bin dir %q", binDir)
	}

	if defaultVersionStr != "" {
		defaultVersion, err := version.NewVersion(defaultVersionStr)
		if err != nil {
			return nil, err
		}
		finalDefaultVersion = defaultVersion
		go func() {
			// Since ensureVersion might end up downloading terraform,
			// we call it asynchronously so as to not delay server startup.
			versionsLock.Lock()
			_, err := ensureVersion(log, tfDownloader, versions, defaultVersion, binDir, tfDownloadURL)
			versionsLock.Unlock()
			if err != nil {
				log.Err("could not download terraform %s: %s", defaultVersion.String(), err)
			}
		}()
	}

	// If tfeToken is set, we try to create a ~/.terraformrc file.
	if tfeToken != "" {
		home, err := homedir.Dir()
		if err != nil {
			return nil, errors.Wrap(err, "getting home dir to write ~/.terraformrc file")
		}
		if err := generateRCFile(tfeToken, tfeHostname, home); err != nil {
			return nil, err
		}
	}

	// We will run terraform with the TF_PLUGIN_CACHE_DIR env var set to this
	// directory inside our data dir.
	cacheDir := filepath.Join(dataDir, terraformPluginCacheDirName)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, errors.Wrapf(err, "unable to create terraform plugin cache directory at %q", terraformPluginCacheDirName)
	}

	return &DefaultClient{
		defaultVersion:          finalDefaultVersion,
		terraformPluginCacheDir: cacheDir,
		binDir:                  binDir,
		downloader:              tfDownloader,
		downloadBaseURL:         tfDownloadURL,
		versionsLock:            &versionsLock,
		versions:                versions,
		usePluginCache:          usePluginCache,
		terraformOutputChan:     terraformOutputChan,
	}, nil
}

// Version returns the default version of Terraform we use if no other version
// is defined.
func (c *DefaultClient) DefaultVersion() *version.Version {
	return c.defaultVersion
}

// TerraformBinDir returns the directory where we download Terraform binaries.
func (c *DefaultClient) TerraformBinDir() string {
	return c.binDir
}

// See Client.EnsureVersion.
func (c *DefaultClient) EnsureVersion(log *logging.SimpleLogger, v *version.Version) error {
	if v == nil {
		v = c.defaultVersion
	}

	var err error
	c.versionsLock.Lock()
	_, err = ensureVersion(log, c.downloader, c.versions, v, c.binDir, c.downloadBaseURL)
	c.versionsLock.Unlock()
	if err != nil {
		return err
	}

	return nil
}

// See Client.RunCommandWithVersion.
func (c *DefaultClient) RunCommandWithVersion(ctx models.ProjectCommandContext, path string, args []string, customEnvVars map[string]string, v *version.Version) (string, error) {
	_, outCh := c.RunCommandAsync(ctx, path, args, customEnvVars, v)
	var lines []string
	var err error
	for line := range outCh {
		if line.Err != nil {
			err = line.Err
			break
		}
		lines = append(lines, line.Line)
	}
	output := strings.Join(lines, "\n")
	return output, err
}

// prepCmd builds a ready to execute command based on the version of terraform
// v, and args. It returns a printable representation of the command that will
// be run and the actual command.
func (c *DefaultClient) prepCmd(log *logging.SimpleLogger, v *version.Version, workspace string, path string, args []string) (string, *exec.Cmd, error) {
	if v == nil {
		v = c.defaultVersion
	}

	var binPath string
	if c.overrideTF != "" {
		// This is only set during testing.
		binPath = c.overrideTF
	} else {
		var err error
		c.versionsLock.Lock()
		binPath, err = ensureVersion(log, c.downloader, c.versions, v, c.binDir, c.downloadBaseURL)
		c.versionsLock.Unlock()
		if err != nil {
			return "", nil, err
		}
	}

	// We add custom variables so that if `extra_args` is specified with env
	// vars then they'll be substituted.
	envVars := []string{
		// Will de-emphasize specific commands to run in output.
		"TF_IN_AUTOMATION=true",
		// Cache plugins so terraform init runs faster.
		fmt.Sprintf("WORKSPACE=%s", workspace),
		fmt.Sprintf("ATLANTIS_TERRAFORM_VERSION=%s", v.String()),
		fmt.Sprintf("DIR=%s", path),
	}
	if c.usePluginCache {
		envVars = append(envVars, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", c.terraformPluginCacheDir))
	}
	// Append current Atlantis process's environment variables, ex.
	// AWS_ACCESS_KEY.
	envVars = append(envVars, os.Environ()...)
	tfCmd := fmt.Sprintf("%s %s", binPath, strings.Join(args, " "))
	cmd := exec.Command("sh", "-c", tfCmd)
	cmd.Dir = path
	cmd.Env = envVars
	return tfCmd, cmd, nil
}

// Line represents a line that was output from a terraform command.
type Line struct {
	// Line is the contents of the line (without the newline).
	Line string
	// Err is set if there was an error.
	Err error
}

// RunCommandAsync runs terraform with args. It immediately returns an
// input and output channel. Callers can use the output channel to
// get the realtime output from the command.
// Callers can use the input channel to pass stdin input to the command.
// If any error is passed on the out channel, there will be no
// further output (so callers are free to exit).
func (c *DefaultClient) RunCommandAsync(ctx models.ProjectCommandContext, path string, args []string, customEnvVars map[string]string, v *version.Version) (chan<- string, <-chan Line) {
	outCh := make(chan Line)
	inCh := make(chan string)

	// We start a goroutine to do our work asynchronously and then immediately
	// return our channels.
	go func() {

		// Ensure we close our channels when we exit.
		defer func() {
			close(outCh)
			close(inCh)
		}()

		tfCmd, cmd, err := c.prepCmd(ctx.Log, v, ctx.Workspace, path, args)
		if err != nil {
			ctx.Log.Err(err.Error())
			outCh <- Line{Err: err}
			return
		}
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		stdin, _ := cmd.StdinPipe()
		envVars := cmd.Env
		for key, val := range customEnvVars {
			envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
		}
		cmd.Env = envVars

		ctx.Log.Debug("starting %q in %q", tfCmd, path)
		err = cmd.Start()
		if err != nil {
			err = errors.Wrapf(err, "running %q in %q", tfCmd, path)
			ctx.Log.Err(err.Error())
			outCh <- Line{Err: err}
			return
		}

		// If we get anything on inCh, write it to stdin.
		// This function will exit when inCh is closed which we do in our defer.
		go func() {
			for line := range inCh {
				ctx.Log.Debug("writing %q to remote command's stdin", line)
				_, err := io.WriteString(stdin, line)
				if err != nil {
					ctx.Log.Err(errors.Wrapf(err, "writing %q to process", line).Error())
				}
			}
		}()

		// Use a waitgroup to block until our stdout/err copying is complete.
		wg := new(sync.WaitGroup)
		wg.Add(2)

		// Asynchronously copy from stdout/err to outCh.
		go func() {
			s := bufio.NewScanner(stdout)
			for s.Scan() {
				outCh <- Line{Line: s.Text()}
				c.terraformOutputChan <- &models.TerraformOutputLine{
					PullSlug: ctx.PullSlug(),
					Line:     s.Text(),
				}
			}
			wg.Done()
		}()
		go func() {
			s := bufio.NewScanner(stderr)
			for s.Scan() {
				outCh <- Line{Line: s.Text()}
				c.terraformOutputChan <- &models.TerraformOutputLine{
					PullSlug: ctx.PullSlug(),
					Line:     s.Text(),
				}
			}
			wg.Done()
		}()

		// Wait for our copying to complete. This *must* be done before
		// calling cmd.Wait(). (see https://github.com/golang/go/issues/19685)
		wg.Wait()

		// Wait for the command to complete.
		err = cmd.Wait()

		// We're done now. Send an error if there was one.
		if err != nil {
			err = errors.Wrapf(err, "running %q in %q", tfCmd, path)
			ctx.Log.Err(err.Error())
			outCh <- Line{Err: err}
		} else {
			ctx.Log.Info("successfully ran %q in %q", tfCmd, path)
		}
	}()

	return inCh, outCh
}

// MustConstraint will parse one or more constraints from the given
// constraint string. The string must be a comma-separated list of
// constraints. It panics if there is an error.
func MustConstraint(v string) version.Constraints {
	c, err := version.NewConstraint(v)
	if err != nil {
		panic(err)
	}
	return c
}

// ensureVersion returns the path to a terraform binary of version v.
// It will download this version if we don't have it.
func ensureVersion(log *logging.SimpleLogger, dl Downloader, versions map[string]string, v *version.Version, binDir string, downloadURL string) (string, error) {
	if binPath, ok := versions[v.String()]; ok {
		return binPath, nil
	}

	// This tf version might not yet be in the versions map even though it
	// exists on disk. This would happen if users have manually added
	// terraform{version} binaries. In this case we don't want to re-download.
	binFile := "terraform" + v.String()
	if binPath, err := exec.LookPath(binFile); err == nil {
		versions[v.String()] = binPath
		return binPath, nil
	}

	// The version might also not be in the versions map if it's in our bin dir.
	// This could happen if Atlantis was restarted without losing its disk.
	dest := filepath.Join(binDir, binFile)
	if _, err := os.Stat(dest); err == nil {
		versions[v.String()] = dest
		return dest, nil
	}
	log.Info("could not find terraform version %s in PATH or %s, downloading from %s", v.String(), binDir, downloadURL)
	urlPrefix := fmt.Sprintf("%s/terraform/%s/terraform_%s", downloadURL, v.String(), v.String())
	binURL := fmt.Sprintf("%s_%s_%s.zip", urlPrefix, runtime.GOOS, runtime.GOARCH)
	checksumURL := fmt.Sprintf("%s_SHA256SUMS", urlPrefix)
	fullSrcURL := fmt.Sprintf("%s?checksum=file:%s", binURL, checksumURL)
	if err := dl.GetFile(dest, fullSrcURL); err != nil {
		return "", errors.Wrapf(err, "downloading terraform version %s at %q", v.String(), fullSrcURL)
	}

	log.Info("downloaded terraform %s to %s", v.String(), dest)
	versions[v.String()] = dest
	return dest, nil
}

// generateRCFile generates a .terraformrc file containing config for tfeToken
// and hostname tfeHostname.
// It will create the file in home/.terraformrc.
func generateRCFile(tfeToken string, tfeHostname string, home string) error {
	const rcFilename = ".terraformrc"
	rcFile := filepath.Join(home, rcFilename)
	config := fmt.Sprintf(rcFileContents, tfeHostname, tfeToken)

	// If there is already a .terraformrc file and its contents aren't exactly
	// what we would have written to it, then we error out because we don't
	// want to overwrite anything.
	if _, err := os.Stat(rcFile); err == nil {
		currContents, err := ioutil.ReadFile(rcFile) // nolint: gosec
		if err != nil {
			return errors.Wrapf(err, "trying to read %s to ensure we're not overwriting it", rcFile)
		}
		if config != string(currContents) {
			return fmt.Errorf("can't write TFE token to %s because that file has contents that would be overwritten", rcFile)
		}
		// Otherwise we don't need to write the file because it already has
		// what we need.
		return nil
	}

	if err := ioutil.WriteFile(rcFile, []byte(config), 0600); err != nil {
		return errors.Wrapf(err, "writing generated %s file with TFE token to %s", rcFilename, rcFile)
	}
	return nil
}

func getVersion(tfBinary string) (*version.Version, error) {
	versionOutBytes, err := exec.Command(tfBinary, "version").Output() // #nosec
	versionOutput := string(versionOutBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "running terraform version: %s", versionOutput)
	}
	match := versionRegex.FindStringSubmatch(versionOutput)
	if len(match) <= 1 {
		return nil, fmt.Errorf("could not parse terraform version from %s", versionOutput)
	}
	return version.NewVersion(match[1])
}

// rcFileContents is a format string to be used with Sprintf that can be used
// to generate the contents of a ~/.terraformrc file for authenticating with
// Terraform Enterprise.
var rcFileContents = `credentials "%s" {
  token = %q
}`

type DefaultDownloader struct{}

// See go-getter.GetFile.
func (d *DefaultDownloader) GetFile(dst, src string, opts ...getter.ClientOption) error {
	return getter.GetFile(dst, src, opts...)
}
