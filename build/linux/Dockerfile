# The Linux build and test machine (NFR-XPLAT-003), run by `make linux-*` through
# Docker. Ubuntu 22.04 for the oldest common glibc, so the binary runs on it and
# anything newer; WebKitGTK 4.1 (Wails' gtk3 build tag), which Ubuntu 22.04+,
# Debian 12+ and Fedora ship. Go and Node are pinned to what the Mac build uses.
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
# The source indexes (deb-src) of the same archive and pockets are fetched with the
# packages and kept, the binary ones not: they name the source of exactly the
# versions installed here, however long the image is kept, for the archive of what
# the AppImage carries (build/linux/sources.sh, OQ-059).
RUN sed -n 's/^deb /deb-src /p' /etc/apt/sources.list > /etc/apt/sources.list.d/deb-src.list \
 && apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git build-essential pkg-config file xz-utils \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
      bubblewrap dbus dbus-x11 gnome-keyring libsecret-tools openssh-client \
      xvfb xauth \
    && rm -f /var/lib/apt/lists/*_Packages*

# Checked against the SHA-256 the projects publish (go.dev/dl, nodejs.org SHASUMS256.txt).
ARG GO_VERSION=1.27.4
ARG GO_SHA256_amd64=7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
ARG GO_SHA256_arm64=3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92
ARG NODE_VERSION=22.22.1
ARG NODE_SHA256_amd64=9a6bc82f9b491279147219f6a18add1e18424dce90d41d2a5fcd69d4924ba3aa
ARG NODE_SHA256_arm64=0f3550d58d45e5d3cf7103d9e3f69937f09fe82fb5dd474c66a5d816fa58c9ee
RUN arch=$(dpkg --print-architecture); nodearch=$([ "$arch" = arm64 ] && echo arm64 || echo x64) \
 && go_sha=$([ "$arch" = arm64 ] && echo "$GO_SHA256_arm64" || echo "$GO_SHA256_amd64") \
 && node_sha=$([ "$arch" = arm64 ] && echo "$NODE_SHA256_arm64" || echo "$NODE_SHA256_amd64") \
 && curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" \
 && echo "${go_sha}  /tmp/go.tgz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && curl -fsSLo /tmp/node.txz "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${nodearch}.tar.xz" \
 && echo "${node_sha}  /tmp/node.txz" | sha256sum -c - \
 && tar -C /usr/local --strip-components=1 -xJf /tmp/node.txz \
 && rm /tmp/go.tgz /tmp/node.txz
ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH GOFLAGS=-buildvcs=false

# Packaging (make linux-package): the pinned wails3, whose nfpm writes the .deb and
# which drives linuxdeploy for the AppImage, and linuxdeploy itself at a fixed
# release rather than the moving "continuous" build wails3 would download. Both
# land outside GOPATH, which the Makefile mounts a cache volume over.
ARG WAILS_VERSION=v3.0.0
RUN GOBIN=/usr/local/bin go install -tags gtk3 github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION} \
 && go clean -cache -modcache
# Checked against the SHA-256 GitHub records for the release assets.
ARG LINUXDEPLOY_VERSION=1-alpha-20251107-1
ARG LINUXDEPLOY_SHA256_amd64=c20cd71e3a4e3b80c3483cef793cda3f4e990aca14014d23c544ca3ce1270b4d
ARG LINUXDEPLOY_SHA256_arm64=620095110d693282b8ebeb244a95b5e911cf8f65f76c88b4b47d16ae6346fcff
RUN arch=$(dpkg --print-architecture); ldarch=$([ "$arch" = arm64 ] && echo aarch64 || echo x86_64) \
 && ld_sha=$([ "$arch" = arm64 ] && echo "$LINUXDEPLOY_SHA256_arm64" || echo "$LINUXDEPLOY_SHA256_amd64") \
 && mkdir -p /opt/linuxdeploy \
 && curl -fsSLo /opt/linuxdeploy/linuxdeploy.AppImage "https://github.com/linuxdeploy/linuxdeploy/releases/download/${LINUXDEPLOY_VERSION}/linuxdeploy-${ldarch}.AppImage" \
 && echo "${ld_sha}  /opt/linuxdeploy/linuxdeploy.AppImage" | sha256sum -c - \
 && chmod 755 /opt/linuxdeploy/linuxdeploy.AppImage
# The AppImage runtime, the program in front of every AppImage and the first code a
# user runs, pinned: left to itself appimagetool downloads the newest from the
# runtime's continuous release at each build (issue 078). Checked against the SHA-256
# GitHub records for the release assets; the AppImage steps take it from here
# (build/linux/Taskfile.yml, APPIMAGE_RUNTIME).
ARG APPIMAGE_RUNTIME_VERSION=20251108
ARG APPIMAGE_RUNTIME_SHA256_amd64=2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
ARG APPIMAGE_RUNTIME_SHA256_arm64=00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444
RUN arch=$(dpkg --print-architecture); rtarch=$([ "$arch" = arm64 ] && echo aarch64 || echo x86_64) \
 && rt_sha=$([ "$arch" = arm64 ] && echo "$APPIMAGE_RUNTIME_SHA256_arm64" || echo "$APPIMAGE_RUNTIME_SHA256_amd64") \
 && curl -fsSLo /opt/linuxdeploy/runtime "https://github.com/AppImage/type2-runtime/releases/download/${APPIMAGE_RUNTIME_VERSION}/runtime-${rtarch}" \
 && echo "${rt_sha}  /opt/linuxdeploy/runtime" | sha256sum -c - \
 && chmod 644 /opt/linuxdeploy/runtime

# pasta (package passt), which gives the sandbox's localhost the host's loopback
# ports (OQ-037). Ubuntu 22.04 has none, so it is built from the source of Ubuntu
# 24.04's, the version the sandbox is pinned to; the SHA-256 is the one in that
# package's .dsc. It lands in /usr/local/bin, where the sandbox looks for it.
ARG PASST_VERSION=0.0~git20240220.1e6f92b
ARG PASST_SHA256=a83098c04140939045c5a5906cab12285fb0e9339c890cb1007af840af8f397f
RUN curl -fsSLo /tmp/passt.txz "http://archive.ubuntu.com/ubuntu/pool/universe/p/passt/passt_${PASST_VERSION}.orig.tar.xz" \
 && echo "${PASST_SHA256}  /tmp/passt.txz" | sha256sum -c - \
 && mkdir /tmp/passt && tar -C /tmp/passt --strip-components=1 -xJf /tmp/passt.txz \
 && make -C /tmp/passt -j"$(nproc)" passt pasta \
 && install -m 0755 /tmp/passt/passt /usr/local/bin/passt && ln -s passt /usr/local/bin/pasta \
 && rm -rf /tmp/passt /tmp/passt.txz

# A user to run as: tests that check a file is unreadable mean nothing as root.
RUN useradd -m -u 1000 build && mkdir -p /home/build/go /home/build/.cache && chown -R build:build /home/build
USER build
ENV HOME=/home/build GOPATH=/home/build/go GOCACHE=/home/build/.cache/go-build
# Tests commit in scratch repositories, as a developer's machine would allow.
RUN git config --global user.name build && git config --global user.email build@localhost
WORKDIR /src
