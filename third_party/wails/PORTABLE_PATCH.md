# Local Wails runtime patch

Source: `github.com/wailsapp/wails/v2` v2.16.0, retaining the upstream MIT license.
The runtime source is pinned here so normal Go and Wails builds apply the same patch;
the module cache is never modified. The upstream CLI, test runner and tooling directories
are not bundled. Use the installed upstream Wails CLI to build this application.

The runtime changes add `mac.Options.NonPersistentWebview`, pass it through the
native preferences struct and selects `WKWebsiteDataStore.nonPersistentDataStore` before
constructing the main WKWebView. Other applications/default options keep upstream behavior.
Linux has the same opt-in, constructing an ephemeral WebKitGTK context before the view.
This avoids global WebKit state and shared localStorage without runtime method replacement.

Files changed from upstream:

- `pkg/options/mac/mac.go`
- `internal/frontend/desktop/darwin/window.go`
- `internal/frontend/desktop/darwin/WailsContext.h`
- `internal/frontend/desktop/darwin/WailsContext.m`
- `pkg/options/linux/linux.go`
- `internal/frontend/desktop/linux/window.go`
- `internal/frontend/desktop/linux/window.h`
- `internal/frontend/desktop/linux/window.c`

Business browser directory storage is implemented separately in this application's
`pkg/browsersession/native_darwin.m`. It uses WebKit's directory configuration SPI,
checks the selectors before use and catches construction failures. The SPI definition
is published in the upstream [WebKit configuration header](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/API/Cocoa/_WKWebsiteDataStoreConfiguration.h).
It requires macOS 15.2+; do not substitute a default or identifier store if unsupported.
Future WebKit/Wails upgrades must recheck the adapter and desktop compilation.

Verification: on macOS 15.7.3, exercised the actual native creation function with a
synthetic cookie: save, separate-process restore, another directory empty, then copy
the profile to a different path and restore successfully. No seller credentials or
Takealot requests were used. Windows production cross-compilation is checked; native
Windows multi-window verification remains a separate platform check.
