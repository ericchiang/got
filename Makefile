.PHONY: build
build:
	bazel build //:got

.PHONY: update
update:
	bazel run //:gazelle

.PHONY: test
test:
	bazel test \
		--test_output=all \
		--test_arg="-test.v" \
		//...

.PHONY: vendor
vendor:
	glide up -v
	glide-vc --only-code --no-tests
	bazel run //:gazelle
