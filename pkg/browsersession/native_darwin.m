//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import "native_darwin.h"

@interface TABrowserView : NSObject <WKNavigationDelegate, WKUIDelegate>
@property WKWebView *view;
@property NSString *origin;
@end
@implementation TABrowserView
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))handler {
    NSURL *url=action.request.URL;
    NSString *origin=[NSString stringWithFormat:@"%@://%@",url.scheme,url.host];
    BOOL frame=action.targetFrame && !action.targetFrame.mainFrame;
    BOOL allowed=[origin isEqualToString:self.origin] || (frame && [url.absoluteString isEqualToString:@"about:blank"]) || (frame && [url.scheme isEqualToString:@"https"] && [url.host isEqualToString:@"challenges.cloudflare.com"]);
    handler(allowed && !url.user && (!url.port || url.port.intValue==443) ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}
- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features { return nil; }
@end
static NSMutableDictionary<NSString *,TABrowserView *> *browserViews;
static __weak WKWebView *applicationView;
static void onMain(void (^block)(void)) { if(NSThread.isMainThread)block();else dispatch_sync(dispatch_get_main_queue(),block); }
static WKWebView *findWebView(NSView *parent) {
    if([parent isKindOfClass:WKWebView.class])return (WKWebView *)parent;
    for(NSView *child in parent.subviews){WKWebView *found=findWebView(child);if(found)return found;}
    return nil;
}
char *browser_start(void) {
    __block char *error=NULL;
    onMain(^{
        if(@available(macOS 14.0,*)){}else{error=strdup("持久会话需要 macOS 14 或以上");return;}
        for(NSWindow *window in NSApp.windows){if([window.title isEqualToString:@"Takealot 掌柜"]){applicationView=findWebView(window.contentView);break;}}
        if(!applicationView){error=strdup("未找到主程序网页视图");return;}
        browserViews=[NSMutableDictionary new];
    });
    return error;
}
char *browser_create(const char *identifier, const char *profile, const char *origin, const char *initialURL) {
    NSString *key=@(identifier),*path=@(profile),*expected=@(origin),*initial=@(initialURL);
    __block char *error=NULL;
    onMain(^{
        if(!applicationView){error=strdup("主窗口尚未就绪");return;}
        TABrowserView *entry=browserViews[key];
        if(!entry){
            NSString *hex=path.lastPathComponent;
            if(hex.length!=32){error=strdup("无效的会话标识");return;}
            NSString *uuid=[NSString stringWithFormat:@"%@-%@-%@-%@-%@",[hex substringWithRange:NSMakeRange(0,8)],[hex substringWithRange:NSMakeRange(8,4)],[hex substringWithRange:NSMakeRange(12,4)],[hex substringWithRange:NSMakeRange(16,4)],[hex substringWithRange:NSMakeRange(20,12)]];
            WKWebViewConfiguration *configuration=[WKWebViewConfiguration new];
            if(@available(macOS 14.0,*)){configuration.websiteDataStore=[WKWebsiteDataStore dataStoreForIdentifier:[[NSUUID alloc] initWithUUIDString:uuid]];}
            // Dedicated views have no Wails scripts, script message handlers,
            // URL scheme handlers, or access to the local application bridge.
            entry=[TABrowserView new];entry.origin=expected;
            entry.view=[[WKWebView alloc] initWithFrame:NSMakeRect(0,0,800,500) configuration:configuration];
            entry.view.navigationDelegate=entry;entry.view.UIDelegate=entry;
            entry.view.hidden=YES;
            [applicationView.superview addSubview:entry.view positioned:NSWindowAbove relativeTo:applicationView];
            browserViews[key]=entry;
        }
        [entry.view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:initial]]];
    });
    return error;
}
void browser_eval(uint64_t token, const char *identifier, const char *source) {
    NSString *key=@(identifier),*script=@(source);
    dispatch_async(dispatch_get_main_queue(),^{
        if(!browser_can_execute(token))return;
        TABrowserView *entry=browserViews[key];
        if(!entry){browser_result(token,"","会话未就绪");return;}
        NSString *origin=[NSString stringWithFormat:@"%@://%@",entry.view.URL.scheme,entry.view.URL.host];
        if(![origin isEqualToString:entry.origin]){browser_result(token,"","会话页面尚未就绪");return;}
        if(@available(macOS 14.0,*)) { [entry.view callAsyncJavaScript:script arguments:@{} inFrame:nil inContentWorld:WKContentWorld.defaultClientWorld completionHandler:^(id value,NSError *error){
            if(error || ![value isKindOfClass:NSString.class])browser_result(token,"","浏览器执行失败");
            else browser_result(token,(char *)[(NSString *)value UTF8String],"");
        }]; } else { browser_result(token,"","不支持的系统版本"); }
    });
}
char *browser_show(const char *identifier,double x,double y,double width,double height,double viewportWidth,double viewportHeight) {
    NSString *key=@(identifier);__block char *error=NULL;
    onMain(^{
        TABrowserView *entry=browserViews[key];
        if(!entry || !applicationView){error=strdup("会话尚未就绪");return;}
        CGFloat sx=applicationView.bounds.size.width/viewportWidth,sy=applicationView.bounds.size.height/viewportHeight;
        CGFloat top=applicationView.isFlipped ? y*sy : applicationView.bounds.size.height-(y+height)*sy;
        NSRect local=NSMakeRect(x*sx,top,width*sx,height*sy);
        entry.view.frame=[applicationView convertRect:local toView:applicationView.superview];
        entry.view.hidden=NO;
    });return error;
}
void browser_hide(void){onMain(^{for(TABrowserView *entry in browserViews.allValues)entry.view.hidden=YES;});}
void browser_destroy(const char *identifier){NSString *key=@(identifier);onMain(^{TABrowserView *entry=browserViews[key];[entry.view stopLoading];[entry.view removeFromSuperview];[browserViews removeObjectForKey:key];});}
void browser_close(void){dispatch_async(dispatch_get_main_queue(),^{for(TABrowserView *entry in browserViews.allValues){[entry.view stopLoading];[entry.view removeFromSuperview];}[browserViews removeAllObjects];});}
