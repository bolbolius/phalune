.PHONY: all build shell settings test clean tidy ui clean-ui install uninstall

export CGO_ENABLED := 1
export PKG_CONFIG_PATH := $(CURDIR)/compat/pkgconfig:$(PKG_CONFIG_PATH)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
DESTDIR ?=

VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

BLP_FILES := $(shell find ui -name "*.blp")
UI_FILES := $(BLP_FILES:.blp=.ui)

all: build

%.ui: %.blp
	blueprint-compiler compile $< --output $@

ui: $(UI_FILES)

clean-ui:
	rm -f $(UI_FILES)

build: ui
	go build -ldflags "$(LDFLAGS)" -o phalune ./cmd/phalune
	go build -ldflags "$(LDFLAGS)" -o phalune-settings ./cmd/phalune-settings

shell: ui
	go build -ldflags "$(LDFLAGS)" -o phalune ./cmd/phalune

settings: ui
	go build -ldflags "$(LDFLAGS)" -o phalune-settings ./cmd/phalune-settings

test: ui
	go test -v ./...


tidy:
	go mod tidy

clean: clean-ui
	rm -f phalune phalune-settings

install: build
	install -Dm755 phalune $(DESTDIR)$(BINDIR)/phalune
	install -Dm755 phalune-settings $(DESTDIR)$(BINDIR)/phalune-settings
	install -Dm644 data/org.phalune.shell.desktop $(DESTDIR)$(DATADIR)/applications/org.phalune.shell.desktop
	install -Dm644 data/icons/hicolor/128x128/apps/phalune.png $(DESTDIR)$(DATADIR)/icons/hicolor/128x128/apps/phalune.png
	install -Dm644 data/icons/hicolor/256x256/apps/phalune.png $(DESTDIR)$(DATADIR)/icons/hicolor/256x256/apps/phalune.png
	install -Dm644 data/icons/hicolor/512x512/apps/phalune.png $(DESTDIR)$(DATADIR)/icons/hicolor/512x512/apps/phalune.png

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/phalune $(DESTDIR)$(BINDIR)/phalune-settings
	rm -f $(DESTDIR)$(DATADIR)/applications/org.phalune.shell.desktop
	rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/128x128/apps/phalune.png
	rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/256x256/apps/phalune.png
	rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/512x512/apps/phalune.png
