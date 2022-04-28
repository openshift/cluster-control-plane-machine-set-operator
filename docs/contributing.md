# Contributing

This document outlines the expectations set out for contributions to this project.

## Conventions

### Comments

Comments are expected on everything.

Comments are expected on constants and global variables; public and private functions; complex areas of code.

A good constant/variable comment should explain what the value is used for.
A good function comment should explain the behaviour of the function, the expected inputs and outputs, any caveats
about the function and notes about any particularly complex parts of the code.

### Errors

Errors are an important part of the user experience and are key to telling users what went wrong and how to fix it.
They are also important programatically for determining actions to take in certain scenarios.
Errors should follow the following conventions:

* Error strings must not start with a capital, nor may they end with punctuation
* Child errors should always be wrapped at the end with `: %w`
* Error messages should refer to Kubernetes resources in the lower case, with spaces logically at the breaks between words
  * This is a convention set out as part of the Red Hat documentation guidelines
  * Eg a `ControlPlaneMachineSet` resource should be referred to as a `control plane machine set` in logs and error
    messages

### Linting

This project has strict linting rules, enforced in CI and run by [golangci-lint](https://golangci-lint.run/).
The linter can be run using `make lint` from the root of the project.

Please make sure the linting rules are passing before submitting a PR.

If the linting seems to be too extreme, please speak to the project maintainers who may be accepting of a
suggestion to make the linting rules more relaxed.

### Logging

This project uses [go-logr](https://github.com/go-logr/logr) to provide structured logging throughout the codebase.
Logging is an important tool and we consider it a part of the user experience, as such, we expect all logging to be
consistent and follow the following guidelines:

* Log messages should start with a capital and should not end with punctuation
* Log messages should be constant
  * No format strings, variable data should be injected as keys and values
  * Make the log string a `const` at the top of the file in which it is used, this makes them easily discoverable
* Log messages should refer to Kubernetes resources in the lower case, with spaces logically at the breaks between words
  * This is a convention set out as part of the Red Hat documentation guidelines
  * Eg a `ControlPlaneMachineSet` resource should be referred to as a `control plane machine set` in logs and error
    messages
* Loggers are scoped to a particular action and should not be embedded in an object, pass them as a variable
  * This allows you to add context which can be added to future log lines
  * Eg. When logs refer to an update strategy, we expect all logs to contain a key `UpdateStrategy` and the value set to
    the update strategy type. This can be achieved by creating a child logger when the update strategy is determined
    ```golang
    logger = logger.WithValues("UpdateStrategy", cpms.Spec.Strategy.Type)
    ```
* Logger keys and values should be lowerCamelCase (as per [Kubernetes Guidelines](https://github.com/kubernetes/community/blob/HEAD/contributors/devel/sig-instrumentation/migration-to-structured-logging.md#name-arguments))

### Testing

Testing is important and we expect all code submissions to have appropriate unit test coverage.
We run unit tests on each PR and the results will include code coverage data to help you identify where your
contributions are missing coverage.

The tests in this project are written with [Ginkgo](https://onsi.github.io/ginkgo/) and
[Gomega](https://onsi.github.io/gomega/) and we expect new contributions to stick to these tools and to write
tests consistent with the rest of the test suite.

If you are new to this style of testing, please reach out to the maintainers of the project who will help you get
up to speed.

Tests can be run using `make test` from the root of the project.
