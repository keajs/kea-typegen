import { kea } from 'kea'
import { typedForm } from './typedForm'
import type { typedFormDemoLogicType } from './typedFormDemoLogicType'

export const typedFormDemoLogic = kea<typedFormDemoLogicType>([typedForm('someinput')])
