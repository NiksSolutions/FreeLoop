package environment

// farPaths: Windows reaches \\server\share and device paths through any file call.
const farPaths = true

// dosNames: Windows reads CON, NUL, COM1 and the like as devices in any folder, takes
// .git. and .git::$INDEX_ALLOCATION for .git and GIT~1 for its short name.
const dosNames = true
