# kea-typegen

Automatically generate types for `kea()` calls for [Kea 2.2+](https://kea.js.org/)

## Docs

https://kea.js.org/docs/guide/typescript/

## Demo

See kea-typegen in action in this [12min screencast](https://www.youtube.com/watch?v=jGy-p9UxcBA).

It's the best documentation currently available :).

## Samples

See [the samples folder](https://github.com/keajs/kea-typegen/tree/master/samples) in this repository.

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
  typegen.ts write  - write logicType.ts files
  typegen.ts watch  - watch for changes and write logicType.ts files

Options:
  --version     Show version number                                    [boolean]
  --config, -c  Path to tsconfig.json (otherwise auto-detected)         [string]
  --file, -f    Single file to evaluate (can't be used with --config)   [string]
  --path, -p    Root for logic paths. E.g: ./frontend/src               [string]
  --quiet, -q   Write nothing to stdout                                [boolean]
  --verbose     Slightly more verbose output log                       [boolean]
  --help        Show help                                              [boolean]
```

