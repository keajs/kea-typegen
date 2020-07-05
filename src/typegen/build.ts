import * as ts from "typescript";
import fs from "fs";
import {ParsedLogic} from "./types";

// function buildLogicType(fileName: string, parsedLogics: ParsedLogic[]) {
//     const printer = ts.createPrinter({newLine: ts.NewLineKind.LineFeed})
//
//
//     for (const parsedLogic of parsedLogics) {
//         const logicType = createLogicType(parsedLogic)
//         const result = printer.printNode(ts.EmitHint.Unspecified, logicType, resultFile)
//     }
//     console.log(result)
//     fs.writeFileSync(resultFile.fileName, result)
//
//     printer.printFile(resultFile)
//     // checker.typeToString(logicType)
//     // debugger
// }
