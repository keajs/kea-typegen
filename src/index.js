import { Project, StructureKind } from 'ts-morph'

const configPath = '/Users/marius/Projects/Kea/kea/tsconfig.json'
const logicPath = '/Users/marius/Projects/Kea/kea/src/__tsplayground__/tstest4.ts'

// initialize
const project = new Project({
  tsConfigFilePath: 'path/to/tsconfig.json',
})
