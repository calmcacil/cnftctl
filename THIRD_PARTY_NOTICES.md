# Third-Party Notices

`cnftctl` is licensed under Apache-2.0. The Go module directly depends on:

- `gopkg.in/yaml.v3`, Copyright 2011-2016 Canonical Ltd., licensed under Apache License 2.0 and MIT terms as distributed by that project.

Release artifacts may invoke operating-system components including nftables, systemd, coreutils, dpkg, and POSIX shell utilities. Those programs are not bundled as project source dependencies and retain their own licenses.

The authoritative dependency set is `go.mod` and `go.sum`. Release review must regenerate or verify this notice when dependencies or bundled assets change. Full third-party license texts remain available from their upstream projects and Debian package metadata.
