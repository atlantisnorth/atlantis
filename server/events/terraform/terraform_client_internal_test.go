package terraform

import (
	"fmt"
	"github.com/hashicorp/go-version"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/runatlantis/atlantis/testing"
)

// Test that we write the file as expected
func TestGenerateRCFile_WritesFile(t *testing.T) {
	tmp, cleanup := TempDir(t)
	defer cleanup()

	err := generateRCFile("token", tmp)
	Ok(t, err)

	expContents := `credentials "app.terraform.io" {
  token = "token"
}`
	actContents, err := ioutil.ReadFile(filepath.Join(tmp, ".terraformrc"))
	Ok(t, err)
	Equals(t, expContents, string(actContents))
}

// Test that if the file already exists and its contents will be modified if
// we write our config that we error out.
func TestGenerateRCFile_WillNotOverwrite(t *testing.T) {
	tmp, cleanup := TempDir(t)
	defer cleanup()

	rcFile := filepath.Join(tmp, ".terraformrc")
	err := ioutil.WriteFile(rcFile, []byte("contents"), 0600)
	Ok(t, err)

	actErr := generateRCFile("token", tmp)
	expErr := fmt.Sprintf("can't write TFE token to %s because that file has contents that would be overwritten", tmp+"/.terraformrc")
	ErrEquals(t, expErr, actErr)
}

// Test that if the file already exists and its contents will NOT be modified if
// we write our config that we don't error.
func TestGenerateRCFile_NoErrIfContentsSame(t *testing.T) {
	tmp, cleanup := TempDir(t)
	defer cleanup()

	rcFile := filepath.Join(tmp, ".terraformrc")
	contents := `credentials "app.terraform.io" {
  token = "token"
}`
	err := ioutil.WriteFile(rcFile, []byte(contents), 0600)
	Ok(t, err)

	err = generateRCFile("token", tmp)
	Ok(t, err)
}

// Test that if we can't read the existing file to see if the contents will be
// the same that we just error out.
func TestGenerateRCFile_ErrIfCannotRead(t *testing.T) {
	tmp, cleanup := TempDir(t)
	defer cleanup()

	rcFile := filepath.Join(tmp, ".terraformrc")
	err := ioutil.WriteFile(rcFile, []byte("can't see me!"), 0000)
	Ok(t, err)

	expErr := fmt.Sprintf("trying to read %s to ensure we're not overwriting it: open %s: permission denied", rcFile, rcFile)
	actErr := generateRCFile("token", tmp)
	ErrEquals(t, expErr, actErr)
}

// Test that if we can't write, we error out.
func TestGenerateRCFile_ErrIfCannotWrite(t *testing.T) {
	rcFile := "/this/dir/does/not/exist/.terraformrc"
	expErr := fmt.Sprintf("writing generated .terraformrc file with TFE token to %s: open %s: no such file or directory", rcFile, rcFile)
	actErr := generateRCFile("token", "/this/dir/does/not/exist")
	ErrEquals(t, expErr, actErr)
}

// Test that it executes with the expected env vars.
func TestDefaultClient_RunCommandWithVersion_EnvVars(t *testing.T) {
	v, err := version.NewVersion("0.11.11")
	Ok(t, err)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	client := &DefaultClient{
		defaultVersion:          v,
		terraformPluginCacheDir: tmp,
		tfExecutableName:        "echo",
	}

	args := []string{
		"TF_IN_AUTOMATION=$TF_IN_AUTOMATION",
		"TF_PLUGIN_CACHE_DIR=$TF_PLUGIN_CACHE_DIR",
		"WORKSPACE=$WORKSPACE",
		"ATLANTIS_TERRAFORM_VERSION=$ATLANTIS_TERRAFORM_VERSION",
		"DIR=$DIR",
	}
	out, err := client.RunCommandWithVersion(nil, tmp, args, nil, "workspace")
	Ok(t, err)
	exp := fmt.Sprintf("TF_IN_AUTOMATION=true TF_PLUGIN_CACHE_DIR=%s WORKSPACE=workspace ATLANTIS_TERRAFORM_VERSION=0.11.11 DIR=%s\n", tmp, tmp)
	Equals(t, exp, out)
}

// Test that it returns an error on error.
func TestDefaultClient_RunCommandWithVersion_Error(t *testing.T) {
	v, err := version.NewVersion("0.11.11")
	Ok(t, err)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	client := &DefaultClient{
		defaultVersion:          v,
		terraformPluginCacheDir: tmp,
		tfExecutableName:        "echo",
	}

	args := []string{
		"dying",
		"&&",
		"exit",
		"1",
	}
	out, err := client.RunCommandWithVersion(nil, tmp, args, nil, "workspace")
	ErrEquals(t, fmt.Sprintf(`running "echo dying && exit 1" in %q: exit status 1`, tmp), err)
	// Test that we still get our output.
	Equals(t, "dying\n", out)
}

func TestDefaultClient_RunCommandAsync_Success(t *testing.T) {
	v, err := version.NewVersion("0.11.11")
	Ok(t, err)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	client := &DefaultClient{
		defaultVersion:          v,
		terraformPluginCacheDir: tmp,
		tfExecutableName:        "echo",
	}

	args := []string{
		"TF_IN_AUTOMATION=$TF_IN_AUTOMATION",
		"TF_PLUGIN_CACHE_DIR=$TF_PLUGIN_CACHE_DIR",
		"WORKSPACE=$WORKSPACE",
		"ATLANTIS_TERRAFORM_VERSION=$ATLANTIS_TERRAFORM_VERSION",
		"DIR=$DIR",
	}
	outCh, errCh := client.RunCommandAsync(nil, tmp, args, nil, "workspace")

	out, err := waitChs(outCh, errCh)
	Ok(t, err)
	exp := fmt.Sprintf("TF_IN_AUTOMATION=true TF_PLUGIN_CACHE_DIR=%s WORKSPACE=workspace ATLANTIS_TERRAFORM_VERSION=0.11.11 DIR=%s", tmp, tmp)
	Equals(t, exp, out)
}

func TestDefaultClient_RunCommandAsync_StderrOutput(t *testing.T) {
	v, err := version.NewVersion("0.11.11")
	Ok(t, err)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	client := &DefaultClient{
		defaultVersion:          v,
		terraformPluginCacheDir: tmp,
		tfExecutableName:        "echo",
	}
	outCh, errCh := client.RunCommandAsync(nil, tmp, []string{"stderr", ">&2"}, nil, "workspace")

	out, err := waitChs(outCh, errCh)
	Ok(t, err)
	Equals(t, "stderr", out)
}

func TestDefaultClient_RunCommandAsync_ExitOne(t *testing.T) {
	v, err := version.NewVersion("0.11.11")
	Ok(t, err)
	tmp, cleanup := TempDir(t)
	defer cleanup()
	client := &DefaultClient{
		defaultVersion:          v,
		terraformPluginCacheDir: tmp,
		tfExecutableName:        "echo",
	}
	outCh, errCh := client.RunCommandAsync(nil, tmp, []string{"dying", "&&", "exit", "1"}, nil, "workspace")

	out, err := waitChs(outCh, errCh)
	ErrEquals(t, fmt.Sprintf(`running "echo dying && exit 1" in %q: exit status 1`, tmp), err)
	// Test that we still get our output.
	Equals(t, "dying", out)
}

func waitChs(outCh <-chan string, errCh <-chan error) (string, error) {
	var out []string
	var err error
	for {
		if outCh == nil && errCh == nil {
			break
		}
		select {
		case line, ok := <-outCh:
			if ok {
				out = append(out, line)
			} else {
				outCh = nil
			}
		case e, ok := <-errCh:
			if ok {
				err = e
			} else {
				errCh = nil
			}
		}
	}
	return strings.Join(out, "\n"), err
}
