# LinBoard — make targets (wrapper around scripts/dev.sh)

.PHONY: help build run rebuild clean clean-cache clean-all clean-legacy deps test test-race vet lint setup install install-shortcut icons stop

help:
	@./scripts/dev.sh help

build:
	@./scripts/dev.sh build

run:
	@./scripts/dev.sh run

rebuild:
	@./scripts/dev.sh rebuild

clean:
	@./scripts/dev.sh clean

clean-cache:
	@./scripts/dev.sh clean-cache

clean-all:
	@./scripts/dev.sh clean-all

deps:
	@./scripts/dev.sh deps

test:
	@./scripts/dev.sh test

vet:
	@./scripts/dev.sh vet

lint:
	@./scripts/dev.sh lint

test-race:
	@./scripts/dev.sh test-race

setup:
	@./scripts/dev.sh setup

install:
	@./scripts/dev.sh install

install-shortcut:
	@./scripts/dev.sh install-shortcut

icons:
	@python3 scripts/generate-icons.py

stop:
	@./scripts/dev.sh stop

clean-legacy:
	@./scripts/dev.sh clean-legacy
