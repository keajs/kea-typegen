import {visitProgram} from "./visit";
import {programFromSource} from "./utils";
import {logicToTypeString} from "./print";

export function logicSourceToLogicType(logicSource: string) {
    const program = programFromSource(logicSource)
    const [parsedLogic] = visitProgram(program)
    return logicToTypeString(parsedLogic)
}