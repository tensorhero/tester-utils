current_minor_number := $(shell git tag --list "v1.*" | sort -V | tail -n 1 | cut -c 4-)
next_minor_number := $(shell echo $$(($(current_minor_number)+1)))

release:
	git tag v1.$(next_minor_number).0
	git push origin master v1.$(next_minor_number).0

test:
	go test -v ./...

test_and_watch:
	onchange '**/*' -- go test -v ./...

setup:
	echo "Setup in tester-utils"