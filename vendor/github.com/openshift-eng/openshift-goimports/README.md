
# openshift-goimports
Organizes Go imports according to OpenShift best practices

* [Summary](#summary)
* [Example sorted import block](#example-sorted-import-block)
* [Installation](#installation)
* [Usage](#usage)
* [Examples](#examples)

## <a name='Summary'></a>Summary
Organizes Go imports into the following groups:
 - **standard** - Any of the Go standard library packages
 - **other** - Anything not specifically called out in this list
 - **kubernetes** - Anything that starts with `k8s.io`
 - **openshift** - Anything that starts with `github.com/openshift`
 - **module** - Anything that is part of the current module

## <a name='Examplesortedimportblock'></a>Example sorted import block
```
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	restclient "k8s.io/client-go/rest"

	buildapiv1 "github.com/openshift/api/build/v1"
	buildscheme "github.com/openshift/client-go/build/clientset/versioned/scheme"
	buildclientv1 "github.com/openshift/client-go/build/clientset/versioned/typed/build/v1"
	"github.com/openshift/library-go/pkg/git"
	"github.com/openshift/library-go/pkg/serviceability"
	s2iapi "github.com/openshift/source-to-image/pkg/api"
	s2igit "github.com/openshift/source-to-image/pkg/scm/git"

	bld "github.com/openshift/builder/pkg/build/builder"
	"github.com/openshift/builder/pkg/build/builder/cmd/scmauth"
	"github.com/openshift/builder/pkg/build/builder/timing"
	builderutil "github.com/openshift/builder/pkg/build/builder/util"
	utillog "github.com/openshift/builder/pkg/build/builder/util/log"
	"github.com/openshift/builder/pkg/version"
)
```

## <a name='Installation'></a>Installation
```
# Install using go get
$ go get -u github.com/openshift-eng/openshift-goimports
```

## <a name='Usage'></a>Usage
```
Usage:
  openshift-goimports [flags]

Flags:
  -h, --help                             help for openshift-goimports
  -l, --list                             List files whose imports are not sorted without making changes
  -m, --module string                    The name of the go module. Example: github.com/example-org/example-repo (optional)
  -p, --path string                      The path to the go module to organize. Defaults to the current directory. (default ".") (optional)
  -d, --dry                              Dry run only, do not actually make any changes to files
  -v, --v Level                          number for the log level verbosity
```

## <a name='Examples'></a>Examples

### <a name='ExampleCLIusage'></a>Example CLI usage
*`openshift-goimports` will try to automatically determine the module using the `go.mod` file, if present, at the provided path location.*

```
# Basic usage, command executed against current directory
$ openshift-goimports

# Basic usage with command executed in provided directory
$ openshift-goimports --module github.com/example-org/example-repo --path ~/go/src/example-org/example-repo
```

### <a name='Examplehacktools.gofile'></a>Example hack/tools.go file
This file will ensure that the `github.com/openshift-eng/openshift-goimports` repo is vendored into your project.
```
//go:build tools
// +build tools

package hack

// Add tools that hack scripts depend on here, to ensure they are vendored.
import (
	_ "github.com/openshift-eng/openshift-goimports"
)

```

### <a name='Examplehackverify-imports.shscript'></a>Example hack/verify-imports.sh script
This file will check if there are any go files that need to be formatted. If there are, it will print a list of them, and exit with status one (1), otherwise it will exit with status zero (0). 
```
#!/bin/bash

bad_files=$(go run ./vendor/github.com/openshift-eng/openshift-goimports -m github.com/example/example-repo -l)
if [[ -n "${bad_files}" ]]; then
        echo "!!! openshift-goimports needs to be run on the following files:"
        echo "${bad_files}"
        echo "Try running 'make imports'"
        exit 1
fi
```

### <a name='ExampleMakefilesections'></a>Example Makefile sections
```
imports: ## Organize imports in go files using openshift-goimports. Example: make imports
	go run ./vendor/github.com/openshift-eng/openshift-goimports/ -m github.com/example/example-repo
.PHONY: imports

verify-imports: ## Run import verifications. Example: make verify-imports
	hack/verify-imports.sh
.PHONY: verify-imports
```