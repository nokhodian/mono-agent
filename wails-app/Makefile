APP_BUNDLE = build/bin/Monoes Agent.app
APP_MACOS  = $(APP_BUNDLE)/Contents/MacOS

.PHONY: build run dev clean

# Development mode: hot-reload frontend + browser DevTools
dev:
	~/go/bin/wails dev

build:
	@echo "→ Building monoes CLI..."
	cd .. && go build -o wails-app/build/bin/monoes ./cmd/monoes/
	@echo "→ Building Wails app..."
	~/go/bin/wails build
	@echo "→ Bundling monoes CLI into app..."
	cp build/bin/monoes "$(APP_MACOS)/monoes"
	@echo "✓ Done: $(APP_BUNDLE)"

run: build
	open "$(APP_BUNDLE)"

clean:
	rm -rf build/bin
