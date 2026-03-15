#!/usr/bin/env node

const prettier = require('prettier')

async function formatGeneratedTypegenFiles(files) {
    const formatted = []

    for (const file of files || []) {
        const filePath = String(file?.filePath || '')
        const sourceText = String(file?.sourceText || '')

        if (!filePath) {
            formatted.push({ filePath, sourceText })
            continue
        }

        let nextText = sourceText
        try {
            const options = await prettier.resolveConfig(filePath)
            nextText = await prettier.format(sourceText, {
                ...(options || {}),
                filepath: filePath,
                trailingComma: 'none',
            })
        } catch (error) {
            nextText = sourceText
        }

        formatted.push({ filePath, sourceText: nextText })
    }

    return formatted
}

async function main() {
    const chunks = []
    for await (const chunk of process.stdin) {
        chunks.push(chunk)
    }

    const input = chunks.length > 0 ? Buffer.concat(chunks).toString('utf8') : ''
    const payload = input ? JSON.parse(input) : { files: [] }
    const files = await formatGeneratedTypegenFiles(payload.files)
    process.stdout.write(JSON.stringify({ files }))
}

if (require.main === module) {
    main().catch(() => {
        process.exitCode = 1
    })
}

module.exports = {
    formatGeneratedTypegenFiles,
}
