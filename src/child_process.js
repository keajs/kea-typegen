const { streamWrite, streamEnd, onExit } = require("@rauschma/stringio");
const { chunksToLinesAsync, chomp } = require("@rauschma/stringio");
const { spawn } = require("child_process");

function openExternalProject(tsserver, configPath) {
  const request = JSON.stringify({
    type: "request",
    command: "openExternalProject",
    arguments: {
      projectFileName: configPath,
      rootFiles: [],
      options: {},
    },
  });
  tsserver.stdin.write(request + "\n");
}

function openFile(tsserver, filePath) {
  const request = JSON.stringify({
    type: "request",
    command: "open",
    arguments: { file: filePath },
  });
  tsserver.stdin.write(request + "\n");
}

function quickInfo(tsserver, filePath, line, offset) {
  const request = JSON.stringify({
    type: "request",
    command: "quickinfo",
    arguments: { offset: offset, line: line, file: filePath },
  });
  tsserver.stdin.write(request + "\n");
}

function matches(event, type, command) {
  return (
    event.type === type &&
    (event.command === command || event.event === command)
  );
}

async function main() {
  const configPath = "/Users/marius/Projects/Kea/kea/tsconfig.json";
  const logicPath =
    "/Users/marius/Projects/Kea/kea/src/__tsplayground__/tstest4.ts";

  const tsserver = spawn("tsserver", [], {
    stdio: ["pipe", "pipe", "pipe"],
  });

  tsserver.stdout.on("data", (data) => {
    const events = data
      .toString()
      .split("\n")
      .filter((s) => s[0] === "{");

    // do something with the events
    events.forEach((json) => {
      const event = JSON.parse(json);

      console.log(event);

      if (matches(event, "event", "typingsInstallerPid")) {
        openExternalProject(tsserver, configPath);
      } else if (matches(event, "response", "openExternalProject")) {
        if (event.success) {
          openFile(tsserver, logicPath);
        }
      } else if (matches(event, "event", "configFileDiag")) {
        const request = JSON.stringify({
          type: "request",
          command: "quickinfo",
          // arguments: { offset: 5, line: 4, file: logicPath },
          // arguments: { offset: 18, line: 3, file: logicPath },
          arguments: { offset: 9, line: 62, file: logicPath },
        });
        tsserver.stdin.write(request + "\n");
      } else if (matches(event, "response", "quickinfo")) {
        console.log('!!')
      } else {
      }
    });
  });

  tsserver.stderr.on("data", (data) => {
    console.error(`stderr: ${data}`);
  });

  await onExit(tsserver);

  console.log("### DONE");
}
main();
