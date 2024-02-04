.DEFAULT_GOAL := help

.PHONY: help
help: ## このMakefileのヘルプを表示します
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## CLIツールをビルドして.bin/に出力します
	go build -o bin/freeedom github.com/JINZO631/freeedom

.PHONY: install
install: ## CLIツールをビルドしてインストールします
	go install github.com/JINZO631/freeedom