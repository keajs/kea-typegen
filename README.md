# kea-typegen

Automatically generate types for `kea()` calls.

## Installation

Install globally:

```
npm install -g kea-typegen@latest

kea-typegen <command>
```

Or use with `npx`:

```
npx kea-typegen <command>
```

## Usage 


```text
$ kea-typegen --help
kea-typegen <command>

Commands:
  typegen.ts check  - check what should be done
  typegen.ts write  - write logic.type.ts files
  typegen.ts watch  - watch for changes and write logic.type.ts files

Options:
  --version     Show version number                                    [boolean]
  --config, -c  Path to tsconfig.json (otherwise auto-detected)         [string]
  --file, -f    Single file to evaluate (can't be used with --config)   [string]
  --path, -p    Root for logic paths. E.g: ./frontend/src               [string]
  --quiet, -q   Write nothing to stdout                                [boolean]
  --verbose     Slightly more verbose output log                       [boolean]
  --help        Show help                                              [boolean]
```

