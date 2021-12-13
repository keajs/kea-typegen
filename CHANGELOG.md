## 1.4.2 - 2021-12-13

-   Fix bug introduced with 1.4.1 if your `tsconfig.json` file does not contain a `types` array.

## 1.4.1 - 2021-12-12

-   Adds new option, `ignoreImportPaths`, to specify a list of files or folders that typegen will never import from, when adding imports inside `logicType.ts` files.
-   Automatically skips adding imports for global types, as specified via the `types` array in `tsconfig.json`. Each entry in the array (e.g. `node`) will add an import ignore path for `./node_modules/@types/node`. To revert this, set `"importGlobalTypes": true` inside `.kearc` or via the CLI.

## 1.4.0 - 2021-12-12

-   Support TypeScript 4.5+.
-   Now adds new type imports with `type`, such as: `import type { logicType } from './logicType`.
-   Refactor internals to create nodes newer `ts.factory` methods.

## 1.3.0 - 2021-11-08

-   Write paths into logics with `--write-paths`. Can be configured in `.kearc` with `writePaths: true`

## 1.2.2 - 2021-10-14

-   Loaders: Make the `payload` parameter on the `Success` action optional.

## 1.2.1 - 2021-10-14

-   Improve `kea-loaders` 0.5.0 support

## 1.2.0 - 2021-10-14

-   Support `kea-loaders` 0.5.0, add `payload` to the success action and `errorObject` to the failure action.

## 1.1.6 - 2021-10-07

-   Support reducers with actions like `[otherLogic().actionTypes.bla]`

## 1.1.5 - 2021-07-14

-   [Fix bug](https://github.com/keajs/kea-typegen/issues/28) with string constant types in action properties when connected with `connect`.

## 1.1.4 - 2021-07-13

-   Support declaring `props: {} as SomeInterface`

## 1.1.3 - 2021-07-13

-   Bugfixes

## 1.1.2 - 2021-07-12

-   Export types and `runTypeGen` function

## 1.1.1 - 2021-07-12

-   Fix main field in package.json

## 1.1.0 - 2021-07-12

-   Experimental support for typegen plugins

## 1.0.4 - 2021-06-27

-   Fix regression

## 1.0.3 - 2021-06-27

-   Fix regression

## 1.0.2 - 2021-06-27

-   Use explicitly specified function return types if present for actions and selectors (vs detection via compiler api).
-   This helps with namespaced types like `This.That`, as the namespace information is lost in detected types.

## 1.0.1 - 2021-06-27

-   Republish as 1.0.0 on npm is actually 0.7.2
-   Only import `A` when encountering complex types like `A.B`

## 1.0.0 - 2021-06-26

-   Support auto-importing referenced types into logic types

## 0.7.2 - 2021-05-30

-   Support actions that return payloads with over 16 properties (fix ".. 4 more .." in the generated type).

## 0.7.1 - 2021-04-30

-   Improve error display, condense logs

## 0.7.0 - 2021-04-26

-   Fix various bugs from last version.
-   Run with `--no-import` to skip automatically adding imports to logic types.

## 0.6.2 - 2021-04-25

-   Automatically add `import { logicType } from './logicType'` statements
-   Automatically add the type to `kea<logicType>()`
-   0.6.0 and 0.6.1 had some bugs, don't use.

## 0.5.4 - 2021-03-30

-   Support reducers with selectors as defaults

## 0.5.3 - 2021-02-23

-   Fix type bug with selectors
-   Skip first comment line in generated types when comparing for overwrites
-   Update deps

## 0.5.2 - 2021-02-23

-   Update and clean up copy

## 0.5.1 - 2021-01-22

-   Fix `kea-typegen write` recursive updates

## 0.5.0 - 2021-01-22

-   Fix "Record<...>" style shortened types in selectors
-   Add names to selectors

## 0.4.0 - 2020-12-29

-   Fixed support for TypeScript 4.1
