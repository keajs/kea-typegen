// https://github.com/microsoft/TypeScript/wiki/Using-the-Compiler-API

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

  const typeOfActions = actions.getType()
  const symbols = typeOfActions.getProperties()

  // [2] == typeof == ObjectLiteralExpression
  const actionsObject = actions.getChildren()[2]

  actionsObject.getProperties().forEach((property) => {
    const name = property.getName()
    const func = property.getChildren()[2]

    console.log(name, func.getType().getText())
  })
}
