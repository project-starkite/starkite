package manager

// metadataFile is the manifest filename at the root of every module directory.
// For starlark modules it is the author-authored module.yaml (parsed via
// libkite.LoadModuleManifest); for WASM modules it is the WASM manifest.
const metadataFile = "module.yaml"
