// https://github.com/Microsoft/TypeScript/wiki/Using-the-Compiler-API#using-the-type-checker
import * as ts from 'typescript'
import { visitProgram } from '../src/visit/visit'

const program = ts.createProgram(['./bin/input/logic.ts'], {
    target: ts.ScriptTarget.ES5,
    module: ts.ModuleKind.CommonJS,
})

visitProgram(program)
