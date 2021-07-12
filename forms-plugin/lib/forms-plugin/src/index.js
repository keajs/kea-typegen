"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.formsPlugin = void 0;
const formsPlugin = () => {
    return {
        name: 'forms',
        defaults: () => ({
            forms: undefined,
        }),
        buildSteps: {
            forms(logic, input) {
                if (!input.forms) {
                    return;
                }
                logic.extend({ actions: { submitForm: true }, reducers: { form: [{ asd: true }] } });
            },
        },
        buildOrder: {
            forms: { after: 'defaults' },
        },
    };
};
exports.formsPlugin = formsPlugin;
//# sourceMappingURL=index.js.map