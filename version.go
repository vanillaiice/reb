// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Package reb holds the engine's version; the engine itself is in internal/, and the commands
// (rebc, the WebAssembly build) in cmd/.
package reb

// Version is the engine release (its git tag). rebc version and the WebAssembly build report it, and
// consumers store it with what they compiled. Bumped with gover (.gover).
const Version = "v0.7.0"
