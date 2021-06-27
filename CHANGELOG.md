## 1.0.3 - 2021-07-27
- Fix regression

## 1.0.2 - 2021-07-27
- Use explicitly specified function return types if present for actions and selectors (vs detection via compiler api).
- This helps with namespaced types like `This.That`, as the namespace information is lost in detected types.

## 1.0.1 - 2021-07-27
- Republish as 1.0.0 on npm is actually 0.7.2
- Only import `A` when encountering complex types like `A.B`

## 1.0.0 - 2021-07-26
- Support auto-importing referenced types into logic types

## 0.7.2 - 2021-05-30
- Support actions that return payloads with over 16 properties (fix ".. 4 more .." in the generated type).

## 0.7.1 - 2021-04-30
- Improve error display, condense logs

## 0.7.0 - 2021-04-26
- Fix various bugs from last version. 
- Run with `--no-import` to skip automatically adding imports to logic types.

## 0.6.2 - 2021-04-25
- Automatically add `import { logicType } from './logicType'` statements
- Automatically add the type to `kea<logicType>()`
- 0.6.0 and 0.6.1 had some bugs, don't use.

## 0.5.4 - 2021-03-30
- Support reducers with selectors as defaults

## 0.5.3 - 2021-02-23
- Fix type bug with selectors
- Skip first comment line in generated types when comparing for overwrites
- Update deps

## 0.5.2 - 2021-02-23
- Update and clean up copy

## 0.5.1 - 2021-01-22
- Fix `kea-typegen write` recursive updates

## 0.5.0 - 2021-01-22
- Fix "Record<...>" style shortened types in selectors
- Add names to selectors

## 0.4.0 - 2020-12-29
- Fixed support for TypeScript 4.1
