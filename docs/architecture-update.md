# Architecture

## Overview

* Never look at GOPATH.
* Never pull HEAD.
* Always remove unused files.

## Dependency versions

The update command attempts to fill the `vendor` directory of a project with the correct set of packages at the correct version. Packages may be __pinned__ to a version __explicitly__ through a project's manifest file, or __transitively__ through a manifest file in a package the project imports.

The command generally tries to be overly cautious. It's better to error out because of an unpinned package than assume HEAD. To enforce this, all of the following requirements must be met:

* Every package imported by the project MUST be pinned explicitly or transitively.
* Every explicitly pinned package MUST be imported by the project.
* If a package is transitively pinned multiple times to different versions, it MUST be explicitly pinned to break ambiguity.
