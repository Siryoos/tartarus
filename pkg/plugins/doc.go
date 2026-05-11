// Package plugins provides the extensibility framework for the Tartarus platform.
//
// Plugins allow operators and third-party developers to extend the platform
// without modifying core code. A plugin is a shared library (.so) that
// exports standard adapters, declared via a JSON manifest.
//
// # Core Types
//
//   - [Plugin]: Represents a loaded plugin with its manifest and adapters.
//
//   - [Manifest]: Describes a plugin's metadata, capabilities, and the
//     adapter types it provides (Judge, Fury, etc.).
//
//   - [Loader]: Loads a plugin from a .so file and validates its manifest.
//     In production, plugins are loaded via [Loader]; in tests, [LoaderStub]
//     is used to inject mock plugins without filesystem access.
//
//   - [Registry]: Tracks all loaded plugins and provides lookup by name and
//     capability.
//
//   - [JudgeAdapter]: Bridges a plugin's exported Judge implementation to
//     the [judges.Judge] interface.
//
//   - [FuryAdapter]: Bridges a plugin's exported Fury implementation to
//     the [erinyes.Fury] interface.
//
// # Manifest Format
//
// Manifests are JSON files co-located with the .so:
//
//	{
//	    "name": "my-plugin",
//	    "version": "1.0.0",
//	    "capabilities": ["judge", "fury"],
//	    "entrypoint": "my_plugin.so"
//	}
//
// # Known Technical Debt
//
//   - Plugin loading uses Go's plugin package which only works on Linux
//     and macOS with CGO enabled. Windows and static builds are not
//     supported. Consider gRPC-based plugin protocol (like Hashicorp's
//     go-plugin) for cross-platform support.
//
//   - There is no plugin sandboxing. A malicious or buggy plugin can
//     crash the entire Olympus/Hecatoncheir process. Running plugins in
//     a subprocess with a strict IPC protocol would isolate faults.
//
//   - Plugin versioning is declared in the manifest but not enforced.
//     A plugin compiled against an older internal API will silently
//     fail at runtime. A plugin API compatibility layer is needed.
//
//   - Hot-reloading (updating a plugin without restarting the server)
//     is not supported. All plugin changes require a process restart.
package plugins
