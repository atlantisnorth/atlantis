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
// Package testdrive is used by the testdrive command as a quick-start of
// Atlantis.
package testdrive

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/go-github/v31/github"
	"github.com/mitchellh/colorstring"
	"github.com/pkg/errors"
)

var terraformExampleRepoOwner = "runatlantis"
var terraformExampleRepo = "atlantis-example"
var bootstrapDescription = `Welcome to Atlantis testdrive!

This mode sets up Atlantis on a test repo so you can try it out. We will
- fork an example terraform project to your username
- install terraform (if not already in your PATH)
- install ngrok so we can expose Atlantis to GitHub
- start Atlantis

[bold]Press Ctrl-c at any time to exit
`
var pullRequestBody = strings.Replace(`
In this pull request we will learn how to use Atlantis.

1. In a couple of seconds you should see the output of Atlantis automatically running $terraform plan$.

1. You can manually run $plan$ by typing a comment:

    $$$
    atlantis plan
    $$$
    Usually you'll let Atlantis automatically run plan for you though.

1. To see all the comment commands available, type:
    $$$
    atlantis help
    $$$

1. To see the help for a specific command, for example $atlantis plan$, type:
    $$$
    atlantis plan --help
    $$$

1. Atlantis holds a "Lock" on this directory to prevent other pull requests modifying
   the Terraform state until this pull request is merged. To view the lock, go to the Atlantis UI: [http://localhost:4141](http://localhost:4141).
   If you wanted, you could manually delete the plan and lock from the UI if you weren't ready to apply. Instead, we will apply it!

1. To $terraform apply$ this change (which does nothing because it is creating a $null_resource$), type:
    $$$
    atlantis apply
    $$$
    **NOTE:** Because this example isn't using [remote state storage](https://www.terraform.io/docs/state/remote.html) the state will be lost once the pull request is merged. To use Atlantis properly, you **must** be using remote state.

1. Finally, merge the pull request to unlock this directory.

Thank you for trying out Atlantis! Next, try using Atlantis on your own repositories: [www.runatlantis.io/guide/getting-started.html](https://www.runatlantis.io/guide/getting-started.html).`, "$", "`", -1)

// Start begins the testdrive process.
// nolint: errcheck
func Start() error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	colorstring.Println(bootstrapDescription)
	colorstring.Print("\n[bold]github.com username: ")
	fmt.Scanln(&githubUsername)
	if githubUsername == "" {
		return fmt.Errorf("please enter a valid github username")
	}
	colorstring.Println(`
To continue, we need you to create a GitHub personal access token
with [green]"repo" [reset]scope so we can fork an example terraform project.

Follow these instructions to create a token (we don't store any tokens):
[green]https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/#creating-a-token[reset]
- use "atlantis" for the token description
- add "repo" scope
- copy the access token
`)
	// Read github token, check for error later.
	colorstring.Print("[bold]GitHub access token (will be hidden): ")
	githubToken, _ = readPassword()
	tp := github.BasicAuthTransport{
		Username: strings.TrimSpace(githubUsername),
		Password: strings.TrimSpace(githubToken),
	}
	githubClient := &Client{client: github.NewClient(tp.Client()), ctx: context.Background()}

	// Fork terraform example repo.
	colorstring.Println("\n=> forking repo ")
	s.Start()
	if err := githubClient.CreateFork(terraformExampleRepoOwner, terraformExampleRepo); err != nil {
		return errors.Wrapf(err, "forking repo %s/%s", terraformExampleRepoOwner, terraformExampleRepo)
	}
	if !githubClient.CheckForkSuccess(terraformExampleRepoOwner, terraformExampleRepo) {
		return fmt.Errorf("didn't find forked repo %s/%s. fork unsuccessful", terraformExampleRepoOwner, terraformExampleRepoOwner)
	}
	s.Stop()
	colorstring.Println("[green]=> fork completed![reset]")

	// Detect terraform and install it if not installed.
	terraformPath, err := exec.LookPath("terraform")
	if err != nil {
		colorstring.Println("[yellow]=> terraform not found in $PATH.[reset]")
		colorstring.Println("=> downloading terraform ")
		s.Start()
		terraformDownloadURL := fmt.Sprintf("%s/terraform/%s/terraform_%s_%s_%s.zip", hashicorpReleasesURL, terraformVersion, terraformVersion, runtime.GOOS, runtime.GOARCH)
		if err = downloadAndUnzip(terraformDownloadURL, "/tmp/terraform.zip", "/tmp"); err != nil {
			return errors.Wrapf(err, "downloading and unzipping terraform")
		}
		colorstring.Println("[green]=> downloaded terraform successfully![reset]")
		s.Stop()

		err = executeCmd("mv", "/tmp/terraform", "/usr/local/bin/")
		if err != nil {
			return errors.Wrapf(err, "moving terraform binary into /usr/local/bin")
		}
		colorstring.Println("[green]=> installed terraform successfully at /usr/local/bin[reset]")
	} else {
		colorstring.Printf("[green]=> terraform found in $PATH at %s\n[reset]", terraformPath)
	}

	// Detect ngrok and install it if not installed
	ngrokPath, ngrokErr := exec.LookPath("ngrok")
	if ngrokErr != nil {
		colorstring.Println("[yellow]=> ngrok not found in $PATH.[reset]")
		colorstring.Println("=> downloading ngrok")
		s.Start()
		ngrokURL := fmt.Sprintf("%s/ngrok-stable-%s-%s.zip", ngrokDownloadURL, runtime.GOOS, runtime.GOARCH)
		if err = downloadAndUnzip(ngrokURL, "/tmp/ngrok.zip", "/tmp"); err != nil {
			return errors.Wrapf(err, "downloading and unzipping ngrok")
		}
		s.Stop()
		colorstring.Println("[green]=> downloaded ngrok successfully![reset]")
		ngrokPath = "/tmp/ngrok"
	} else {
		colorstring.Printf("[green]=> ngrok found in $PATH at %s\n[reset]", ngrokPath)
	}

	// Create ngrok tunnel.
	colorstring.Println("=> creating secure tunnel")
	s.Start()

	// We use a config file so we can set ngrok's API port (web_addr). We use
	// the API to get the public URL and if there's already ngrok running, it
	// will just choose a random API port and we won't be able to get the right
	// url.
	ngrokConfig := fmt.Sprintf(`
web_addr: %s
tunnels:
  atlantis:
    addr: %d
    bind_tls: true
    proto: http
`, ngrokAPIURL, atlantisPort)

	ngrokConfigFile, err := ioutil.TempFile("", "")
	if err != nil {
		return errors.Wrap(err, "creating ngrok config file")
	}
	err = ioutil.WriteFile(ngrokConfigFile.Name(), []byte(ngrokConfig), 0600)
	if err != nil {
		return errors.Wrap(err, "writing ngrok config file")
	}

	// Used to ensure proper termination of all background commands.
	var wg sync.WaitGroup
	defer wg.Wait()

	tunnelReadyLog := regexp.MustCompile("client session established")
	tunnelTimeout := 20 * time.Second
	cancelNgrok, ngrokErrors, err := execAndWaitForStderr(&wg, tunnelReadyLog, tunnelTimeout,
		ngrokPath, "start", "atlantis", "--config", ngrokConfigFile.Name(), "--log", "stderr", "--log-format", "term")
	// Check if we got a fast error. Move on if we haven't (the command is still running).
	if err != nil {
		s.Stop()
		return errors.Wrap(err, "creating ngrok tunnel")
	}
	// When this function returns, ngrok tunnel should be stopped.
	defer cancelNgrok()

	// The tunnel is up!
	s.Stop()
	colorstring.Println("[green]=> started tunnel![reset]")
	// There's a 1s delay between tunnel starting and API being up.
	time.Sleep(1 * time.Second)
	tunnelURL, err := getTunnelAddr()
	if err != nil {
		return errors.Wrapf(err, "getting tunnel url")
	}

	// Start atlantis server.
	colorstring.Println("=> starting atlantis server")
	s.Start()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.Wrap(err, "creating a temporary data directory for Atlantis")
	}
	defer os.RemoveAll(tmpDir)
	serverReadyLog := regexp.MustCompile("Atlantis started - listening on port 4141")
	serverReadyTimeout := 5 * time.Second
	cancelAtlantis, atlantisErrors, err := execAndWaitForStderr(&wg, serverReadyLog, serverReadyTimeout,
		os.Args[0], "server", "--gh-user", githubUsername, "--gh-token", githubToken, "--data-dir", tmpDir, "--atlantis-url", tunnelURL, "--repo-allowlist", fmt.Sprintf("github.com/%s/%s", githubUsername, terraformExampleRepo))
	// Check if we got a fast error. Move on if we haven't (the command is still running).
	if err != nil {
		return errors.Wrap(err, "creating atlantis server")
	}
	// When this function returns atlantis server should be stopped.
	defer cancelAtlantis()

	colorstring.Printf("[green]=> atlantis server is now securely exposed at [bold][underline]%s\n[reset]", tunnelURL)
	fmt.Println("")

	// Create atlantis webhook.
	colorstring.Println("=> creating atlantis webhook")
	s.Start()
	err = githubClient.CreateWebhook(githubUsername, terraformExampleRepo, fmt.Sprintf("%s/events", tunnelURL))
	if err != nil {
		return errors.Wrapf(err, "creating atlantis webhook")
	}
	s.Stop()
	colorstring.Println("[green]=> atlantis webhook created![reset]")

	// Create a new pr in the example repo.
	colorstring.Println("=> creating a new pull request")
	s.Start()
	pullRequestURL, err := githubClient.CreatePullRequest(githubUsername, terraformExampleRepo, "example", "master")
	if err != nil {
		return errors.Wrapf(err, "creating new pull request for repo %s/%s", githubUsername, terraformExampleRepo)
	}
	s.Stop()
	colorstring.Println("[green]=> pull request created![reset]")

	// Open new pull request in the browser.
	colorstring.Println("=> opening pull request")
	s.Start()
	time.Sleep(2 * time.Second)
	err = executeCmd("open", pullRequestURL)
	if err != nil {
		colorstring.Printf("[red]=> opening pull request failed. please go to: %s on the browser\n[reset]", pullRequestURL)
	}
	s.Stop()

	// Wait for ngrok and atlantis server process to finish.
	colorstring.Println("[_green_][light_green]atlantis is running [reset]")
	s.Start()
	colorstring.Println("[green] [press Ctrl-c to exit][reset]")

	// Wait for SIGINT or SIGTERM signals meaning the user has Ctrl-C'd the
	// testdrive process and want's to stop.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Keep checking for errors from ngrok or atlantis server. Exit normally on shutdown signal.
	select {
	case <-signalChan:
		colorstring.Println("\n[red]shutdown signal received, exiting....[reset]")
		colorstring.Println("\n[green]Thank you for using atlantis :) \n[reset]For more information about how to use atlantis in production go to: https://www.runatlantis.io")
		return nil
	case err := <-ngrokErrors:
		return errors.Wrap(err, "ngrok tunnel")
	case err := <-atlantisErrors:
		return errors.Wrap(err, "atlantis server")
	}
}
