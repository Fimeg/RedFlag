package system

import "testing"

func TestParseCgroupOwner(t *testing.T) {
	const dockerID = "a3f1c2d4e5b60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

	cases := []struct {
		name string
		body string
		want ProcessOwner
	}{
		{
			name: "v2 system service",
			body: "0::/system.slice/nginx.service\n",
			want: ProcessOwner{Cgroup: "/system.slice/nginx.service", Unit: "nginx.service"},
		},
		{
			name: "v2 user scope keeps the innermost unit",
			body: "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-firefox-4090.scope\n",
			want: ProcessOwner{
				Cgroup: "/user.slice/user-1000.slice/user@1000.service/app.slice/app-firefox-4090.scope",
				Unit:   "app-firefox-4090.scope",
			},
		},
		{
			name: "docker systemd driver",
			body: "0::/system.slice/docker-" + dockerID + ".scope\n",
			want: ProcessOwner{
				Cgroup:           "/system.slice/docker-" + dockerID + ".scope",
				ContainerID:      dockerID,
				ContainerRuntime: "docker",
			},
		},
		{
			name: "docker cgroupfs driver",
			body: "0::/docker/" + dockerID + "\n",
			want: ProcessOwner{
				Cgroup:           "/docker/" + dockerID,
				ContainerID:      dockerID,
				ContainerRuntime: "docker",
			},
		},
		{
			name: "podman",
			body: "0::/machine.slice/libpod-" + dockerID + ".scope\n",
			want: ProcessOwner{
				Cgroup:           "/machine.slice/libpod-" + dockerID + ".scope",
				ContainerID:      dockerID,
				ContainerRuntime: "podman",
			},
		},
		{
			name: "cri-containerd under kubepods",
			body: "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podabc.slice/cri-containerd-" + dockerID + ".scope\n",
			want: ProcessOwner{
				Cgroup:           "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podabc.slice/cri-containerd-" + dockerID + ".scope",
				ContainerID:      dockerID,
				ContainerRuntime: "containerd",
			},
		},
		{
			name: "kubepods bare id",
			body: "0::/kubepods/burstable/pod9c4f/" + dockerID + "\n",
			want: ProcessOwner{
				Cgroup:           "/kubepods/burstable/pod9c4f/" + dockerID,
				ContainerID:      dockerID,
				ContainerRuntime: "containerd",
			},
		},
		{
			name: "lxc payload",
			body: "0::/lxc.payload.mail/system.slice/postfix.service\n",
			want: ProcessOwner{
				Cgroup:           "/lxc.payload.mail/system.slice/postfix.service",
				Unit:             "postfix.service",
				ContainerID:      "mail",
				ContainerRuntime: "lxc",
			},
		},
		{
			name: "lxc plain",
			body: "0::/lxc/mail\n",
			want: ProcessOwner{Cgroup: "/lxc/mail", ContainerID: "mail", ContainerRuntime: "lxc"},
		},
		{
			name: "v1 prefers the systemd hierarchy",
			body: "12:pids:/\n" +
				"6:memory:/system.slice\n" +
				"1:name=systemd:/system.slice/sshd.service\n",
			want: ProcessOwner{Cgroup: "/system.slice/sshd.service", Unit: "sshd.service"},
		},
		{
			name: "v1 without systemd falls back to a named controller",
			body: "6:memory:/system.slice/cron.service\n5:cpu:/\n",
			want: ProcessOwner{Cgroup: "/system.slice/cron.service", Unit: "cron.service"},
		},
		{
			name: "kernel thread at the root has no owner",
			body: "0::/\n",
			want: ProcessOwner{},
		},
		{
			name: "empty",
			body: "",
			want: ProcessOwner{},
		},
		{
			name: "malformed lines are skipped",
			body: "garbage\n0::/system.slice/chronyd.service\n",
			want: ProcessOwner{Cgroup: "/system.slice/chronyd.service", Unit: "chronyd.service"},
		},
		{
			name: "a scope that is not a container id stays a unit",
			body: "0::/system.slice/docker-notahash.scope\n",
			want: ProcessOwner{
				Cgroup: "/system.slice/docker-notahash.scope",
				Unit:   "docker-notahash.scope",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCgroupOwner(tc.body)
			if got != tc.want {
				t.Fatalf("parseCgroupOwner(%q)\n got: %+v\nwant: %+v", tc.body, got, tc.want)
			}
		})
	}
}

func TestIsContainerID(t *testing.T) {
	valid := []string{
		"a3f1c2d4e5b60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
		"a3f1c2d4e5b6",
	}
	for _, id := range valid {
		if !isContainerID(id) {
			t.Errorf("isContainerID(%q) = false, want true", id)
		}
	}

	invalid := []string{
		"",
		"nginx.service",
		"A3F1C2D4E5B6", // runtimes emit lowercase; uppercase is something else
		"a3f1c2d4e5b",  // 11
		"a3f1c2d4e5b60", // 13
		"g3f1c2d4e5b6",
	}
	for _, id := range invalid {
		if isContainerID(id) {
			t.Errorf("isContainerID(%q) = true, want false", id)
		}
	}
}
