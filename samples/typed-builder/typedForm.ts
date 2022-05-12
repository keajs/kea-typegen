import { actions, Logic, LogicBuilder, reducers } from "kea";

export function typedForm<L extends Logic = Logic>(input: string): LogicBuilder<L> {
    return (logic) => {
        actions({ submitForm: true })(logic)
        reducers({ form: [{ value: input }] })(logic)
    }
}
