.PHONY: all build shell settings test clean tidy ui clean-ui

export CGO_ENABLED := 1
export PKG_CONFIG_PATH := $(CURDIR)/compat/pkgconfig:$(PKG_CONFIG_PATH)

BLP_FILES := $(shell find ui -name "*.blp")
UI_FILES := $(BLP_FILES:.blp=.ui)

all: build

%.ui: %.blp
	blueprint-compiler compile $< --output $@

ui: $(UI_FILES)

clean-ui:
	rm -f $(UI_FILES)

build: ui
	go build -o phalune ./cmd/phalune
	go build -o phalune-settings ./cmd/phalune-settings

shell: ui
	go build -o phalune ./cmd/phalune

settings: ui
	go build -o phalune-settings ./cmd/phalune-settings

test: ui
	go test -v ./...


tidy:
	go mod tidy

clean: clean-ui
	rm -f phalune phalune-settings
