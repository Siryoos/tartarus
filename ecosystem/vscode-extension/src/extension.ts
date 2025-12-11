import * as vscode from 'vscode';
import { registerCommands } from './commands';
import { TartarusTreeProvider } from './treeProvider';

export function activate(context: vscode.ExtensionContext) {
	console.log('Tartarus extension is now active!');

	// Register Commands
	registerCommands(context);

	// Register Tree Data Provider
	const treeProvider = new TartarusTreeProvider();
	vscode.window.registerTreeDataProvider('tartarus.sandboxes', treeProvider);

	// Register refresh command specifically for the tree view
	context.subscriptions.push(
		vscode.commands.registerCommand('tartarus.refresh', () => treeProvider.refresh())
	);
}

export function deactivate() { }
