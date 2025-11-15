# Persistent Audio Navigation

## Implementation Summary

This site now uses client-side routing to enable persistent audio playback across page navigation.

## How It Works

1. **Content Wrapper**: The main page content is wrapped in `#app-content` div
2. **Persistent Player**: The footer audio player stays outside the wrapper and persists across navigations
3. **Router**: The `router.js` intercepts link clicks and dynamically loads new content
4. **History API**: Browser back/forward buttons work correctly

## Supported Navigation Flows

✅ **Working:**
- Home → Patch page (via link click)
- Patch page → Home (via link click)  
- Browser back/forward buttons
- Audio continues playing during all navigation
- Footer player visible when audio is playing

## Known Limitations

⚠️ **Direct Navigation to Patch Pages:**
When you directly visit a patch page URL (type it in or refresh), the footer player HTML is present but not functional. This is because the player initialization code only runs on the home page.

**Workaround:** Navigate to home first, then to patch pages.

**Future Fix:** Extract player initialization into a shared JavaScript file loaded by all pages.

## Testing Checklist

- [ ] Start audio on home page
- [ ] Navigate to a patch page (audio should continue)
- [ ] Navigate back to home (audio should continue)
- [ ] Use browser back button (audio should continue)
- [ ] Use browser forward button (audio should continue)
- [ ] Stop audio and navigate (footer should hide)
- [ ] Start audio on a patch page... (won't work - see limitations)

## Files Modified

- `public/index.html` - Added app-content wrapper, modularized initialization
- `public/router.js` - New file with client-side routing
- `public/patch/mulch-channel-features.html` - Updated with footer player and wrapper

## Files To Update

The following patch pages need the same treatment as `mulch-channel-features.html`:
- home-cooking-show-10.html
- no-mo-play-in-the-ga.html
- show-notes.html
- tft-1028-tracklist.html
- tft-1111.html
- wart-the-music-of-pete-pete.html

## Next Steps

1. Apply the same wrapper/footer/scripts pattern to all patch pages
2. (Optional) Extract player initialization to shared JS file for full functionality

