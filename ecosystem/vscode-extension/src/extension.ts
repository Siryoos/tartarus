import * as vscode from 'vscode';

export function activate(context: vscode.ExtensionContext) {
	console.log('Tartarus extension is now active!');

	let disposable = vscode.commands.registerCommand('tartarus.helloWorld', () => {
		vscode.window.showInformationMessage('Hello from Tartarus!');
	});

	context.subscriptions.push(disposable);
}

export function deactivate() {}
