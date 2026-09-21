//go:build darwin

#import <Cocoa/Cocoa.h>

extern void termcp_tray_action(int action);

enum {
    TermcpTrayShowWindow = 1,
    TermcpTrayOpenWorkspace = 2,
    TermcpTrayOpenService = 3,
    TermcpTrayStartCore = 4,
    TermcpTrayStopCore = 5,
    TermcpTrayRestartCore = 6,
    TermcpTrayToggleAutostart = 7,
    TermcpTrayShowAbout = 8,
    TermcpTrayQuit = 9,
};

@interface TermcpTrayTarget : NSObject
- (void)performAction:(NSMenuItem *)sender;
@end

@implementation TermcpTrayTarget
- (void)performAction:(NSMenuItem *)sender {
    termcp_tray_action((int)sender.tag);
}
@end

static NSStatusItem *termcpStatusItem;
static TermcpTrayTarget *termcpTrayTarget;
static NSMenuItem *termcpProductItem;
static NSMenuItem *termcpCoreItem;
static NSMenuItem *termcpServiceItem;
static NSMenuItem *termcpShowItem;
static NSMenuItem *termcpWorkspaceItem;
static NSMenuItem *termcpManageServiceItem;
static NSMenuItem *termcpStartItem;
static NSMenuItem *termcpStopItem;
static NSMenuItem *termcpRestartItem;
static NSMenuItem *termcpAutostartItem;
static NSMenuItem *termcpAboutItem;
static NSMenuItem *termcpQuitItem;

static NSMenuItem *termcpActionItem(NSString *title, NSInteger tag) {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title
                                                 action:@selector(performAction:)
                                          keyEquivalent:@""];
    item.target = termcpTrayTarget;
    item.tag = tag;
    return item;
}

void termcp_tray_start(const void *icon, int icon_length) {
    (void)icon;
    (void)icon_length;
    dispatch_async(dispatch_get_main_queue(), ^{
        if (termcpStatusItem != nil) {
            return;
        }
        termcpTrayTarget = [TermcpTrayTarget new];
        termcpStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        termcpStatusItem.button.toolTip = @"Termcp";
        termcpStatusItem.button.image = nil;
        termcpStatusItem.button.title = @"T_";
        if (@available(macOS 10.15, *)) {
            termcpStatusItem.button.font = [NSFont monospacedSystemFontOfSize:13 weight:NSFontWeightSemibold];
        } else {
            termcpStatusItem.button.font = [NSFont boldSystemFontOfSize:13];
        }

        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Termcp"];
        termcpProductItem = [[NSMenuItem alloc] initWithTitle:@"Termcp" action:nil keyEquivalent:@""];
        termcpCoreItem = [[NSMenuItem alloc] initWithTitle:@"Core：正在读取状态" action:nil keyEquivalent:@""];
        termcpServiceItem = [[NSMenuItem alloc] initWithTitle:@"系统服务：正在读取状态" action:nil keyEquivalent:@""];
        termcpProductItem.enabled = NO;
        termcpCoreItem.enabled = NO;
        termcpServiceItem.enabled = NO;
        [menu addItem:termcpProductItem];
        [menu addItem:termcpCoreItem];
        [menu addItem:termcpServiceItem];
        [menu addItem:[NSMenuItem separatorItem]];
        termcpShowItem = termcpActionItem(@"Termcp", TermcpTrayShowWindow);
        termcpWorkspaceItem = termcpActionItem(@"SSH", TermcpTrayOpenWorkspace);
        termcpManageServiceItem = termcpActionItem(@"Service", TermcpTrayOpenService);
        [menu addItem:termcpShowItem];
        [menu addItem:termcpWorkspaceItem];
        [menu addItem:termcpManageServiceItem];
        [menu addItem:[NSMenuItem separatorItem]];
        termcpStartItem = termcpActionItem(@"启动 Core", TermcpTrayStartCore);
        termcpStopItem = termcpActionItem(@"停止 Core", TermcpTrayStopCore);
        termcpRestartItem = termcpActionItem(@"重启 Core", TermcpTrayRestartCore);
        termcpAutostartItem = termcpActionItem(@"开机自启", TermcpTrayToggleAutostart);
        [menu addItem:termcpStartItem];
        [menu addItem:termcpStopItem];
        [menu addItem:termcpRestartItem];
        [menu addItem:termcpAutostartItem];
        [menu addItem:[NSMenuItem separatorItem]];
        termcpAboutItem = termcpActionItem(@"About", TermcpTrayShowAbout);
        termcpQuitItem = termcpActionItem(@"Quit", TermcpTrayQuit);
        [menu addItem:termcpAboutItem];
        [menu addItem:termcpQuitItem];
        termcpStatusItem.menu = menu;
    });
}

void termcp_tray_update(const char *product, const char *core, const char *service,
                        const char *tooltip, const char *show_window,
                        const char *workspace, const char *manage_service,
                        const char *start_core, const char *stop_core,
                        const char *restart_core, const char *autostart_text,
                        const char *about, const char *quit,
                        int core_running, int service_supported,
                        int service_installed, int autostart) {
    NSString *productText = product ? [NSString stringWithUTF8String:product] : @"Termcp";
    NSString *coreText = core ? [NSString stringWithUTF8String:core] : @"Core：未知";
    NSString *serviceText = service ? [NSString stringWithUTF8String:service] : @"系统服务：未知";
    NSString *tooltipText = tooltip ? [NSString stringWithUTF8String:tooltip] : @"Termcp";
    NSString *showWindowText = show_window ? [NSString stringWithUTF8String:show_window] : @"Termcp";
    NSString *workspaceText = workspace ? [NSString stringWithUTF8String:workspace] : @"SSH";
    NSString *manageServiceText = manage_service ? [NSString stringWithUTF8String:manage_service] : @"Service";
    NSString *startCoreText = start_core ? [NSString stringWithUTF8String:start_core] : @"Start Core";
    NSString *stopCoreText = stop_core ? [NSString stringWithUTF8String:stop_core] : @"Stop Core";
    NSString *restartCoreText = restart_core ? [NSString stringWithUTF8String:restart_core] : @"Restart Core";
    NSString *autostartText = autostart_text ? [NSString stringWithUTF8String:autostart_text] : @"Autostart";
    NSString *aboutText = about ? [NSString stringWithUTF8String:about] : @"About Termcp";
    NSString *quitText = quit ? [NSString stringWithUTF8String:quit] : @"Quit Termcp";
    dispatch_async(dispatch_get_main_queue(), ^{
        termcpProductItem.title = productText;
        termcpCoreItem.title = coreText;
        termcpServiceItem.title = serviceText;
        termcpStatusItem.button.toolTip = tooltipText;
        termcpShowItem.title = showWindowText;
        termcpWorkspaceItem.title = workspaceText;
        termcpManageServiceItem.title = manageServiceText;
        termcpStartItem.title = startCoreText;
        termcpStopItem.title = stopCoreText;
        termcpRestartItem.title = restartCoreText;
        termcpAutostartItem.title = autostartText;
        termcpAboutItem.title = aboutText;
        termcpQuitItem.title = quitText;
        termcpStartItem.enabled = !core_running;
        termcpStopItem.enabled = core_running;
        termcpRestartItem.enabled = YES;
        termcpAutostartItem.enabled = service_supported && service_installed;
        termcpAutostartItem.state = autostart ? NSControlStateValueOn : NSControlStateValueOff;
    });
}

void termcp_tray_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (termcpStatusItem != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:termcpStatusItem];
        }
        termcpStatusItem = nil;
        termcpTrayTarget = nil;
        termcpProductItem = nil;
        termcpCoreItem = nil;
        termcpServiceItem = nil;
        termcpShowItem = nil;
        termcpWorkspaceItem = nil;
        termcpManageServiceItem = nil;
        termcpStartItem = nil;
        termcpStopItem = nil;
        termcpRestartItem = nil;
        termcpAutostartItem = nil;
        termcpAboutItem = nil;
        termcpQuitItem = nil;
    });
}
