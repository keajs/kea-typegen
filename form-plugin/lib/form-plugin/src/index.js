"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.formPlugin = void 0;
const formPlugin = () => {
    return {
        name: 'form',
        defaults: () => ({
            form: undefined,
        }),
        buildSteps: {
            form(logic, input) {
                if (!input.form) {
                    return;
                }
                logic.extend({ actions: { submitForm: true }, reducers: { form: [{ asd: true }] } });
            },
        },
        buildOrder: {
            form: { after: 'defaults' },
        },
    };
};
exports.formPlugin = formPlugin;
//# sourceMappingURL=index.js.map