import { kea, path } from 'kea'
import { typedForm } from './typedForm'
import type { typedFormDemoLogicType } from './typedFormDemoLogicType'

export const typedFormDemoLogic = kea<typedFormDemoLogicType>([
    path(['typed-builder', 'typedFormDemoLogic']),
    typedForm('someinput'),
])
