/*-
 * Copyright 2015 Grammarly, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package build

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/grammarly/rocker/src/imagename"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/mock"

	"github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

// =========== Testing FROM ===========

func TestCommandFrom_Existing(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "from",
		args: []string{"existing"},
	})

	img := &docker.Image{
		ID: "123",
		Config: &docker.Config{
			Hostname: "localhost",
		},
	}

	c.On("InspectImage", "existing:latest").Return(img, nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, "123", state.ImageID)
	assert.Equal(t, "localhost", state.Config.Hostname)
}

func TestCommandFrom_NotExisting(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "from",
		args: []string{"not-existing"},
	})

	var nilImg *docker.Image
	var nilList []*imagename.ImageName

	c.On("InspectImage", "not-existing:latest").Return(nilImg, nil).Once()
	c.On("ListImages").Return(nilList, nil).Once()
	c.On("ListImageTags", "not-existing:latest").Return(nilList, nil).Once()

	_, err := cmd.Execute(b)
	c.AssertExpectations(t)
	assert.Equal(t, "FROM error: Image not found: not-existing:latest (also checked in the remote registry)", err.Error())
}

// =========== Testing RUN ===========

func TestCommandRun_Simple(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "run",
		args: []string{"whoami"},
	})

	origCmd := []string{"/bin/program"}
	b.state.Config.Cmd = origCmd
	b.state.ImageID = "123"

	c.On("CreateContainer", mock.AnythingOfType("State")).Return("456", nil).Run(func(args mock.Arguments) {
		arg := args.Get(0).(State)
		assert.Equal(t, []string{"/bin/sh", "-c", "whoami"}, arg.Config.Cmd)
	}).Once()

	c.On("RunContainer", "456", false).Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, origCmd, b.state.Config.Cmd)
	assert.Equal(t, origCmd, state.Config.Cmd)
	assert.Equal(t, "123", state.ImageID)
	assert.Equal(t, "456", state.NoCache.ContainerID)
}

func TestCommandRun_ArgNoEnv(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "run",
		args: []string{"export | grep proxy"},
	})

	b.state.Config.Cmd = []string{"/bin/program"}
	b.state.ImageID = "123"
	b.state.NoCache.BuildArgs = map[string]string{"http_proxy": "http://host:3128"}

	c.On("CreateContainer", mock.AnythingOfType("State")).Return("456", nil).Run(func(args mock.Arguments) {
		arg := args.Get(0).(State)
		assert.Equal(t, []string{"http_proxy=http://host:3128"}, arg.Config.Env)
	}).Once()

	c.On("RunContainer", "456", false).Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, `RUN ["|1" "http_proxy=http://host:3128" "/bin/sh" "-c" "export | grep proxy"]`, state.GetCommits())
	assert.Equal(t, []string(nil), state.Config.Env)
}

func TestCommandRun_ArgWithEnv(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "run",
		args: []string{"export | grep proxy"},
	})

	b.state.Config.Cmd = []string{"/bin/program"}
	b.state.Config.Env = []string{"foo=bar", "lopata=some_value"}
	b.state.ImageID = "123"
	b.state.NoCache.BuildArgs = map[string]string{
		"http_proxy": "http://host:3128",
		"lopata":     "default",
	}

	c.On("CreateContainer", mock.AnythingOfType("State")).Return("456", nil).Run(func(args mock.Arguments) {
		arg := args.Get(0).(State)
		assert.Equal(t, []string{"foo=bar", "lopata=some_value", "http_proxy=http://host:3128"}, arg.Config.Env)
	}).Once()

	c.On("RunContainer", "456", false).Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, `RUN ["|1" "http_proxy=http://host:3128" "/bin/sh" "-c" "export | grep proxy"]`, state.GetCommits())
	assert.Equal(t, []string{"foo=bar", "lopata=some_value"}, state.Config.Env)
}

// =========== Testing COMMIT ===========

func TestCommandCommit_Simple(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := &CommandCommit{}

	resultImage := &docker.Image{ID: "789"}
	b.state.ImageID = "123"
	b.state.NoCache.ContainerID = "456"
	b.state.Commit("a").Commit("b")

	c.On("CommitContainer", mock.AnythingOfType("State")).Return(resultImage, nil).Once()
	c.On("RemoveContainer", "456").Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, "a; b", b.state.GetCommits())
	assert.Equal(t, "", state.GetCommits())
	assert.Equal(t, []string(nil), state.Config.Cmd)
	assert.Equal(t, "789", state.ImageID)
	assert.Equal(t, "", state.NoCache.ContainerID)
}

func TestCommandCommit_NoContainer(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := &CommandCommit{}

	resultImage := &docker.Image{ID: "789"}
	b.state.ImageID = "123"
	b.state.Commit("a").Commit("b")

	c.On("CreateContainer", mock.AnythingOfType("State")).Return("456", nil).Run(func(args mock.Arguments) {
		arg := args.Get(0).(State)
		assert.Equal(t, []string{"/bin/sh", "-c", "#(nop) a; b"}, arg.Config.Cmd)
	}).Once()

	c.On("CommitContainer", mock.AnythingOfType("State")).Return(resultImage, nil).Once()
	c.On("RemoveContainer", "456").Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, "a; b", b.state.GetCommits())
	assert.Equal(t, "", state.GetCommits())
	assert.Equal(t, "789", state.ImageID)
	assert.Equal(t, "", state.NoCache.ContainerID)
}

func TestCommandCommit_NoCommitMsgs(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := &CommandCommit{}

	_, err := cmd.Execute(b)
	assert.Nil(t, err)
}

// TODO: test skip commit

// =========== Testing ENV ===========

func TestCommandEnv_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "env",
		args: []string{"type", "web", "env", "prod"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "ENV type=web env=prod", state.GetCommits())
	assert.Equal(t, []string{"type=web", "env=prod"}, state.Config.Env)
}

func TestCommandEnv_Advanced(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "env",
		args: []string{"type", "web", "env", "prod"},
	})

	b.state.Config.Env = []string{"env=dev", "version=1.2.3"}

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "ENV type=web env=prod", state.GetCommits())
	assert.Equal(t, []string{"env=prod", "version=1.2.3", "type=web"}, state.Config.Env)
}

// =========== Testing LABEL ===========

func TestCommandLabel_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "label",
		args: []string{"type", "web", "env", "prod"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	expectedLabels := map[string]string{
		"type": "web",
		"env":  "prod",
	}

	t.Logf("Result labels: %# v", pretty.Formatter(state.Config.Labels))

	assert.Equal(t, "LABEL type=web env=prod", state.GetCommits())
	assert.True(t, reflect.DeepEqual(state.Config.Labels, expectedLabels), "bad result labels")
}

func TestCommandLabel_Advanced(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "label",
		args: []string{"type", "web", "env", "prod"},
	})

	b.state.Config.Labels = map[string]string{
		"env":     "dev",
		"version": "1.2.3",
	}

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	expectedLabels := map[string]string{
		"type":    "web",
		"version": "1.2.3",
		"env":     "prod",
	}

	t.Logf("Result labels: %# v", pretty.Formatter(state.Config.Labels))

	assert.Equal(t, "LABEL type=web env=prod", state.GetCommits())
	assert.True(t, reflect.DeepEqual(state.Config.Labels, expectedLabels), "bad result labels")
}

// =========== Testing MAINTAINER ===========

func TestCommandMaintainer_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "maintainer",
		args: []string{"terminator"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", state.GetCommits())
}

// =========== Testing WORKDIR ===========

func TestCommandWorkdir_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "workdir",
		args: []string{"/app"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "/app", state.Config.WorkingDir)
}

func TestCommandWorkdir_Relative_HasRoot(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "workdir",
		args: []string{"www"},
	})

	b.state.Config.WorkingDir = "/home"

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "/home/www", state.Config.WorkingDir)
}

func TestCommandWorkdir_Relative_NoRoot(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "workdir",
		args: []string{"www"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "/www", state.Config.WorkingDir)
}

// =========== Testing CMD ===========

func TestCommandCmd_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "cmd",
		args: []string{"apt-get", "install"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"/bin/sh", "-c", "apt-get install"}, state.Config.Cmd)
}

func TestCommandCmd_Json(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name:  "cmd",
		args:  []string{"apt-get", "install"},
		attrs: map[string]bool{"json": true},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"apt-get", "install"}, state.Config.Cmd)
}

// =========== Testing ENTRYPOINT ===========

func TestCommandEntrypoint_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "entrypoint",
		args: []string{"/bin/sh"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"/bin/sh", "-c", "/bin/sh"}, state.Config.Entrypoint)
}

func TestCommandEntrypoint_Json(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name:  "entrypoint",
		args:  []string{"/bin/bash", "-c"},
		attrs: map[string]bool{"json": true},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"/bin/bash", "-c"}, state.Config.Entrypoint)
}

func TestCommandEntrypoint_Remove(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "entrypoint",
		args: []string{},
	})

	b.state.Config.Entrypoint = []string{"/bin/sh", "-c"}

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{}, state.Config.Entrypoint)
}

// =========== Testing EXPOSE ===========

func TestCommandExpose_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "expose",
		args: []string{"80"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	expectedPorts := map[docker.Port]struct{}{
		docker.Port("80/tcp"): struct{}{},
	}

	assert.True(t, reflect.DeepEqual(expectedPorts, state.Config.ExposedPorts), "bad exposed ports")
}

func TestCommandExpose_Add(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "expose",
		args: []string{"443"},
	})

	b.state.Config.ExposedPorts = map[docker.Port]struct{}{
		docker.Port("80/tcp"): struct{}{},
	}

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	expectedPorts := map[docker.Port]struct{}{
		docker.Port("80/tcp"):  struct{}{},
		docker.Port("443/tcp"): struct{}{},
	}

	assert.True(t, reflect.DeepEqual(expectedPorts, state.Config.ExposedPorts), "bad exposed ports")
}

// =========== Testing VOLUME ===========

func TestCommandVolume_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "volume",
		args: []string{"/data"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	volumes := map[string]struct{}{
		"/data": struct{}{},
	}

	assert.True(t, reflect.DeepEqual(volumes, state.Config.Volumes), "bad volumes")
}

func TestCommandVolume_Add(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "volume",
		args: []string{"/var/log"},
	})

	b.state.Config.Volumes = map[string]struct{}{
		"/data": struct{}{},
	}

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	volumes := map[string]struct{}{
		"/data":    struct{}{},
		"/var/log": struct{}{},
	}

	assert.True(t, reflect.DeepEqual(volumes, state.Config.Volumes), "bad volumes")
}

// =========== Testing USER ===========

func TestCommandUser_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "user",
		args: []string{"www"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "www", state.Config.User)
}

// =========== Testing ONBUILD ===========

func TestCommandOnBuild_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name:     "onbuild",
		args:     []string{"RUN", "make", "install"},
		original: "ONBUILD RUN make install",
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, []string{"RUN make install"}, state.Config.OnBuild)
}

// =========== Testing COPY ===========

func TestCommandCopy_Simple(t *testing.T) {
	// TODO: do we need to check the dest is always a directory?
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "copy",
		args: []string{"testdata/Rockerfile", "/Rockerfile"},
	})

	c.On("CreateContainer", mock.AnythingOfType("State")).Return("456", nil).Run(func(args mock.Arguments) {
		arg := args.Get(0).(State)
		// TODO: a better check
		assert.True(t, len(arg.Config.Cmd) > 0)
	}).Once()

	c.On("UploadToContainer", "456", mock.AnythingOfType("*io.PipeReader"), "/").Return(nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("state: %# v", pretty.Formatter(state))

	c.AssertExpectations(t)
	assert.Equal(t, "456", state.NoCache.ContainerID)
}

// =========== Testing TAG ===========

func TestCommandTag_Simple(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "tag",
		args: []string{"docker.io/grammarly/rocker:1.0"},
	})

	b.state.ImageID = "123"

	c.On("TagImage", "123", "docker.io/grammarly/rocker:1.0").Return(nil).Once()

	_, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
}

func TestCommandTag_WrongArgsNumber(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "tag",
		args: []string{},
	})
	cmd2 := NewCommand(ConfigCommand{
		name: "tag",
		args: []string{"1", "2"},
	})

	b.state.ImageID = "123"

	_, err := cmd.Execute(b)
	assert.EqualError(t, err, "TAG requires exactly one argument")

	_, err2 := cmd2.Execute(b)
	assert.EqualError(t, err2, "TAG requires exactly one argument")
}

func TestCommandTag_NoImage(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "tag",
		args: []string{"docker.io/grammarly/rocker:1.0"},
	})

	_, err := cmd.Execute(b)
	assert.EqualError(t, err, "Cannot TAG on empty image")
}

// =========== Testing PUSH ===========

func TestCommandPush_Simple(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "push",
		args: []string{"docker.io/grammarly/rocker:1.0"},
	})

	b.cfg.Push = true
	b.state.ImageID = "123"

	c.On("TagImage", "123", "docker.io/grammarly/rocker:1.0").Return(nil).Once()
	c.On("PushImage", "docker.io/grammarly/rocker:1.0").Return("sha256:fafa", nil).Once()

	_, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
}

func TestCommandPush_WrongArgsNumber(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "push",
		args: []string{},
	})
	cmd2 := NewCommand(ConfigCommand{
		name: "push",
		args: []string{"1", "2"},
	})

	b.state.ImageID = "123"

	_, err := cmd.Execute(b)
	assert.EqualError(t, err, "PUSH requires exactly one argument")

	_, err2 := cmd2.Execute(b)
	assert.EqualError(t, err2, "PUSH requires exactly one argument")
}

func TestCommandPush_NoImage(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "push",
		args: []string{"docker.io/grammarly/rocker:1.0"},
	})

	_, err := cmd.Execute(b)
	assert.EqualError(t, err, "Cannot PUSH empty image")
}

// =========== Testing MOUNT ===========

func TestCommandMount_Simple(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "mount",
		args: []string{"/src:/dest"},
	})

	c.On("ResolveHostPath", "/src").Return("/resolved/src", nil).Once()

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	c.AssertExpectations(t)
	assert.Equal(t, []string{"/resolved/src:/dest"}, state.NoCache.HostConfig.Binds)
	assert.Equal(t, `MOUNT ["/src:/dest"]`, state.GetCommits())
}

func TestCommandMount_VolumeContainer(t *testing.T) {
	b, c := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "mount",
		args: []string{"/cache"},
	})

	containerName := b.mountsContainerName("/cache")

	c.On("EnsureContainer", containerName, mock.AnythingOfType("*docker.Config"), mock.AnythingOfType("*docker.HostConfig"), "/cache").Return("123", nil).Run(func(args mock.Arguments) {
		arg := args.Get(1).(*docker.Config)
		assert.Equal(t, MountVolumeImage, arg.Image)
		expectedVolumes := map[string]struct{}{
			"/cache": struct{}{},
		}
		assert.True(t, reflect.DeepEqual(expectedVolumes, arg.Volumes))
	}).Once()

	cnt := &docker.Container{
		Name: "/" + containerName,
		Mounts: []docker.Mount{
			{
				Source:      "/volumedir",
				Destination: "/cache",
			},
		},
	}

	c.On("InspectContainer", containerName).Return(cnt, nil)

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	commitMsg := fmt.Sprintf("MOUNT [\"%s:/cache\"]", containerName)

	c.AssertExpectations(t)
	assert.Equal(t, []string{"/volumedir:/cache:ro"}, state.NoCache.HostConfig.Binds)
	assert.Equal(t, commitMsg, state.GetCommits())
}

// =========== Testing ARG ===========

func TestCommandArg_Simple(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "arg",
		args: []string{"foo=bar"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, b.allowedBuildArgs["foo"])
	assert.Equal(t, "bar", state.NoCache.BuildArgs["foo"])
	assert.Equal(t, "ARG foo=bar", state.GetCommits())
}

func TestCommandArg_Allow(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "arg",
		args: []string{"xxx"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, b.allowedBuildArgs["xxx"])
	assert.NotContains(t, state.NoCache.BuildArgs, "xxx")
	assert.Equal(t, "ARG xxx", state.GetCommits())
}

func TestCommandArg_Substitution(t *testing.T) {
	b, _ := makeBuild(t, "", Config{})
	cmd := NewCommand(ConfigCommand{
		name: "arg",
		args: []string{"arg1=arg1val"},
	})

	state, err := cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, b.allowedBuildArgs["arg1"])
	assert.Equal(t, "arg1val", state.NoCache.BuildArgs["arg1"])
	assert.Equal(t, "ARG arg1=arg1val", state.GetCommits())

	cmd = NewCommand(ConfigCommand{
		name: "arg",
		args: []string{"arg2=arg2val"},
	})

	state, err = cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, b.allowedBuildArgs["arg2"])
	assert.Equal(t, "arg2val", state.NoCache.BuildArgs["arg2"])
	assert.Equal(t, "ARG arg2=arg2val", state.GetCommits())

	cmd = NewCommand(ConfigCommand{
		name: "arg",
		args: []string{"argcompose=start$arg1 middle${arg2}end"},
	})

	state, err = cmd.Execute(b)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, true, b.allowedBuildArgs["argcompose"])
	assert.Equal(t, "startarg1val middlearg2valend", state.NoCache.BuildArgs["argcompose"])
}

// TODO: test Cleanup
