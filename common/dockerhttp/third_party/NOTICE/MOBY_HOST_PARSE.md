# Attribution: Docker host URL parsing

Portions of host URL parsing (`ParseHostURL`, `DefaultDockerHost`,
`DOCKER_HOST` / `DOCKER_API_VERSION` env handling) are adapted from:

  github.com/moby/moby/client @ v25.0.6
  https://github.com/moby/moby/tree/v25.0.6/client

Copyright 2013-2018 Docker, Inc.
Licensed under the Apache License, Version 2.0.
See APACHE-2.0.txt in this directory.

Only the minimal host-parse / env / default-socket logic was adapted.
The full Docker Engine SDK is NOT vendored.

## Darwin / Docker Desktop note

Official `DefaultDockerHost` on macOS remains `unix:///var/run/docker.sock`.
Docker Desktop may expose the engine at `unix://${HOME}/.docker/run/docker.sock`
(desktop-linux context) and optionally symlink `/var/run/docker.sock`.
This module’s Darwin `resolveDefaultHost` probes those two paths when
`DOCKER_HOST` is unset; it does not parse `~/.docker/contexts`.
