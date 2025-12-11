"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.deactivate = exports.activate = void 0;
const vscode = require("vscode");
const commands_1 = require("./commands");
const treeProvider_1 = require("./treeProvider");
function activate(context) {
    console.log('Tartarus extension is now active!');
    // Register Commands
    (0, commands_1.registerCommands)(context);
    // Register Tree Data Provider
    const treeProvider = new treeProvider_1.TartarusTreeProvider();
    vscode.window.registerTreeDataProvider('tartarus.sandboxes', treeProvider);
    // Register refresh command specifically for the tree view
    context.subscriptions.push(vscode.commands.registerCommand('tartarus.refresh', () => treeProvider.refresh()));
}
exports.activate = activate;
function deactivate() { }
exports.deactivate = deactivate;
//# sourceMappingURL=extension.js.map