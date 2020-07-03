// TODO: https://levelup.gitconnected.com/writing-typescript-custom-ast-transformer-part-1-7585d6916819


// https://github.com/microsoft/TypeScript/wiki/Using-the-Compiler-API
// https://dev.doctorevidence.com/how-to-write-a-typescript-transform-plugin-fc5308fdd943

const { Project, StructureKind, ts } = require('ts-morph')

const configPath = '/Users/marius/Projects/Kea/kea/tsconfig.json'
const logicPath = '/Users/marius/Projects/Kea/kea/src/__tsplayground__/tstest4.ts'

// initialize
const project = new Project({
  tsConfigFilePath: configPath,
})
const checker = project.getTypeChecker()

const source = project.getSourceFileOrThrow(logicPath)

// call expressions
for (const callExpression of source.getDescendantsOfKind(ts.SyntaxKind.CallExpression)) {
  const keaInput = callExpression.getArguments()[0]

  const actions = keaInput.getProperty('actions')

  const actionsType = actions.getType()
  const prop2 = actionsType.getProperty(p => p.getName() === "goSomewhere");

  const symbol = prop2.compilerSymbol


  // const symbols = typeOfActions.getProperties()
  // const symbol = symbols[0]
  // const member = actionsType.compilerType.members

  // const member = [...typeOfActions.compilerType.members][0][1]

  ts.isClassDeclaration(prop2)

  console.log('!')

  //
  // // [2] == typeof == ObjectLiteralExpression
  // const actionsObject = actions.getChildren()[2]
  // const actionsType = actionsObject.getType()
  //
  // actionsObject.getProperties().forEach((property) => {
  //   const name = property.getName()
  //   const func = property.getChildren()[2]
  //
  //   console.log(name, func.getType().getText())
  // })
}
