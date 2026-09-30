//go:build darwin

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void TermcpHandleDockReopen(void);

// Wails 2.15 keeps the process alive after the main window is hidden but does
// not implement applicationShouldHandleReopen. Add the method at runtime so
// this package does not need to link directly against Wails' Objective-C class.
static BOOL termcpShouldHandleReopen(id self, SEL command,
                                     NSApplication *application,
                                     BOOL hasVisibleWindows) {
    TermcpHandleDockReopen();
    return YES;
}

int TermcpInstallDockReopenHandler(void) {
    Class delegate = NSClassFromString(@"AppDelegate");
    if (delegate == Nil) {
        return 0;
    }
    SEL selector = @selector(applicationShouldHandleReopen:hasVisibleWindows:);
    if (class_getInstanceMethod(delegate, selector) != NULL) {
        return 1;
    }
    return class_addMethod(delegate, selector, (IMP)termcpShouldHandleReopen,
                           "c@:@c") ? 1 : 0;
}
