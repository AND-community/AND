import * as vscode from "vscode";
import { listGroups, listProjects } from "./groupProjectStore";
import { Grup, Proje } from "./types";
import { t } from "./dil/dil";

export class GroupItem extends vscode.TreeItem {
  contextValue = "deltaxGroup";
  constructor(readonly grup: Grup) {
    super(grup.ad, vscode.TreeItemCollapsibleState.Expanded);
    this.description = t("%d üye", grup.uyeler.length);
    this.iconPath = new vscode.ThemeIcon("organization");
    this.tooltip = t("Kurucu: %s", grup.kurucu_ad);
  }
}

export class ProjectItem extends vscode.TreeItem {
  contextValue = "deltaxProject";
  constructor(readonly proje: Proje, isActive: boolean) {
    super(proje.ad, vscode.TreeItemCollapsibleState.None);
    this.description = proje.aktif_dal;
    this.iconPath = new vscode.ThemeIcon(isActive ? "folder-active" : "folder");
    this.tooltip = isActive ? t("Şu anda VS Code'da açık") : t("Açmak için tıklayın");
    this.command = { command: "deltax.openProject", title: t("Projeyi Aç"), arguments: [this] };
  }
}

/** Sana gelen, henüz yanıtlamadığın davetler: tıklayınca kabul/ret seçimi açılır. */
export class InvitesItem extends vscode.TreeItem {
  contextValue = "deltaxInvites";
  constructor(count: number) {
    super(t("Bekleyen davetler"), vscode.TreeItemCollapsibleState.None);
    this.description = String(count);
    this.iconPath = new vscode.ThemeIcon("mail", new vscode.ThemeColor("notificationsInfoIcon.foreground"));
    this.tooltip = t("Gruba katılmak ya da daveti reddetmek için tıklayın");
    this.command = { command: "deltax.showInvites", title: t("Bekleyen Davetler") };
  }
}

type Node = InvitesItem | GroupItem | ProjectItem;

export class GroupsTreeProvider implements vscode.TreeDataProvider<Node> {
  private readonly changeEmitter = new vscode.EventEmitter<Node | undefined | void>();
  readonly onDidChangeTreeData = this.changeEmitter.event;

  constructor(
    private readonly getRoot: () => string,
    private readonly activeProjectId: () => string | undefined,
    private readonly pendingInviteCount: () => number
  ) {}

  refresh(): void {
    this.changeEmitter.fire();
  }

  getTreeItem(element: Node): vscode.TreeItem {
    return element;
  }

  getChildren(element?: Node): Node[] {
    const root = this.getRoot();
    if (!element) {
      const invites = this.pendingInviteCount();
      const groups: Node[] = listGroups(root).map((g) => new GroupItem(g));
      return invites > 0 ? [new InvitesItem(invites), ...groups] : groups;
    }
    if (element instanceof GroupItem) {
      return listProjects(root, element.grup.id).map(
        (p) => new ProjectItem(p, p.id === this.activeProjectId())
      );
    }
    return [];
  }
}
