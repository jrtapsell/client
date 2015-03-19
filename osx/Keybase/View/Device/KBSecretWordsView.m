//
//  KBSecretWordsView.m
//  Keybase
//
//  Created by Gabriel on 3/17/15.
//  Copyright (c) 2015 Gabriel Handford. All rights reserved.
//

#import "KBSecretWordsView.h"

@interface KBSecretWordsView ()
@property KBLabel *label;
@property KBLabel *secretWordsLabel;
@end

@implementation KBSecretWordsView

- (void)viewInit {
  [super viewInit];

  YOView *contentView = [[YOView alloc] init];
  [self addSubview:contentView];

  KBLabel *header = [[KBLabel alloc] init];
  [header setText:@"Register Device" style:KBLabelStyleHeaderLarge alignment:NSCenterTextAlignment lineBreakMode:NSLineBreakByTruncatingTail];
  [contentView addSubview:header];

  _label = [[KBLabel alloc] init];
  [contentView addSubview:_label];

  _secretWordsLabel = [[KBLabel alloc] init];
  _secretWordsLabel.selectable = YES;
  [_secretWordsLabel setBackgroundColor:KBAppearance.currentAppearance.secondaryBackgroundColor];
  [_secretWordsLabel setBorderWithColor:KBAppearance.currentAppearance.lineColor width:1.0]; //cornerRadius:6
  _secretWordsLabel.insets = UIEdgeInsetsMake(10, 20, 10, 20);
  [contentView addSubview:_secretWordsLabel];

  _button = [KBButton buttonWithText:@"OK" style:KBButtonStylePrimary];
  [contentView addSubview:_button];

  YOSelf yself = self;
  contentView.viewLayout = [YOLayout layoutWithLayoutBlock:^(id<YOLayout> layout, CGSize size) {
    CGFloat y = 0;

    y += [layout centerWithSize:CGSizeMake(400, 0) frame:CGRectMake(40, y, size.width - 80, 0) view:header].size.height + 20;

    y += [layout centerWithSize:CGSizeMake(400, 0) frame:CGRectMake(40, y, size.width - 80, 0) view:yself.label].size.height + 30;

    y += [layout centerWithSize:CGSizeMake(400, 0) frame:CGRectMake(40, y, size.width - 80, 0) view:yself.secretWordsLabel].size.height + 40;

    y += [layout centerWithSize:CGSizeMake(200, 0) frame:CGRectMake(40, y, size.width - 80, 0) view:yself.button].size.height;

    return CGSizeMake(MIN(480, size.width), y);
  }];

  self.viewLayout = [YOLayout layoutWithLayoutBlock:[KBLayouts center:contentView]];
}

- (void)setSecretWords:(NSString *)secretWords deviceNameToRegister:(NSString *)deviceNameToRegister {
  // NSStringWithFormat(@"In order to register this device you need to enter in these secret words on the device named: <strong>%@</strong>.", deviceName)
  [_label setMarkup:@"In order to register this device you need to enter in these secret words on your device." style:KBLabelStyleDefault alignment:NSLeftTextAlignment lineBreakMode:NSLineBreakByWordWrapping];
  [_secretWordsLabel setText:[secretWords uppercaseString] font:[NSFont boldSystemFontOfSize:20] color:KBAppearance.currentAppearance.textColor alignment:NSCenterTextAlignment];
  [self setNeedsLayout];
}

@end
