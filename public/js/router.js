// ============================================
// CLIENT-SIDE ROUTER
// Enables persistent audio playback across page navigation
// ============================================

import { getPlayerState, setCurrentPath } from "./state.js";

const Router = {
  // Configuration
  config: {
    contentSelector: "#app-content",
    linkSelector: "a[href]",
    // Elements that should be extracted from new pages
    extractSelectors: {
      content: "#app-content",
      title: "title",
    },
  },

  // State
  isNavigating: false,
  currentPath: window.location.pathname,

  // Initialize the router
  init() {
    // Don't initialize if we're in a navigated state (prevents double init)
    if (window.__routerInitialized) {
      return;
    }
    window.__routerInitialized = true;

    console.log("[Router] Initializing client-side router");

    // Mark initial stylesheets as dynamic for proper management
    this.markInitialStylesheetsAsDynamic();

    // Set initial state
    if (!window.history.state) {
      window.history.replaceState(
        { path: this.currentPath, timestamp: Date.now() },
        document.title,
        this.currentPath
      );
    }

    setCurrentPath(this.currentPath);

    // Listen for link clicks
    document.addEventListener("click", (e) => this.handleLinkClick(e), true);

    // Listen for browser back/forward
    window.addEventListener("popstate", (e) => this.handlePopState(e));

    console.log("[Router] Router initialized successfully");
  },

  // Handle link clicks
  handleLinkClick(e) {
    // Find the clicked link (might be nested in the link)
    const link = e.target.closest("a[href]");

    if (!link) return;

    const href = link.getAttribute("href");

    // Ignore if:
    // - No href
    // - External link
    // - Anchor link only
    // - Download link
    // - Target="_blank" or other target
    // - Modified click (ctrl, cmd, shift, etc.)
    if (
      !href ||
      href.startsWith("http://") ||
      href.startsWith("https://") ||
      href.startsWith("//") ||
      href.startsWith("mailto:") ||
      href.startsWith("tel:") ||
      href === "#" ||
      link.getAttribute("download") !== null ||
      link.target === "_blank" ||
      e.ctrlKey ||
      e.metaKey ||
      e.shiftKey ||
      e.altKey
    ) {
      return;
    }

    // Handle anchor links on same page
    if (href.startsWith("#")) {
      return; // Let browser handle it
    }

    // Prevent default and handle via router
    e.preventDefault();
    e.stopPropagation();

    // Convert relative URLs to absolute paths
    const url = new URL(href, window.location.origin);
    const targetPath = url.pathname;

    // Don't navigate if we're already on this page
    if (targetPath === this.currentPath) {
      console.log("[Router] Already on page:", targetPath);
      return;
    }

    console.log("[Router] Navigating to:", targetPath);
    this.navigate(targetPath);
  },

  // Navigate to a new page
  async navigate(path, skipPushState = false) {
    if (this.isNavigating) {
      console.log("[Router] Navigation already in progress");
      return;
    }

    this.isNavigating = true;

    try {
      console.log("[Router] Loading content from:", path);

      // Fetch the new page
      const html = await this.fetchPage(path);

      // Extract content and metadata
      const extracted = this.extractContent(html);

      // Update the page
      this.updatePage(extracted);

      // Update history
      if (!skipPushState) {
        window.history.pushState(
          { path: path, timestamp: Date.now() },
          extracted.title,
          path
        );
      }

      this.currentPath = path;
      setCurrentPath(path);

      // Scroll to top
      window.scrollTo(0, 0);

      // Re-initialize page-specific scripts
      if (window.reinitPage) {
        console.log("[Router] Re-initializing page-specific scripts");
        window.reinitPage();
      }

      console.log("[Router] Navigation complete");
    } catch (error) {
      console.error("[Router] Navigation failed:", error);
      // Fallback to full page load
      console.log("[Router] Falling back to full page load");
      window.location.href = path;
    } finally {
      this.isNavigating = false;
    }
  },

  // Fetch a page
  async fetchPage(path) {
    const response = await fetch(path, {
      headers: {
        "X-Requested-With": "XMLHttpRequest", // Indicate this is an AJAX request
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return await response.text();
  },

  // Extract content from HTML string
  extractContent(html) {
    // Create a temporary DOM parser
    const parser = new DOMParser();
    const doc = parser.parseFromString(html, "text/html");

    // Extract the main content
    const contentElement = doc.querySelector(
      this.config.extractSelectors.content
    );
    if (!contentElement) {
      throw new Error("Could not find content element in fetched page");
    }

    // Extract title
    const titleElement = doc.querySelector(this.config.extractSelectors.title);
    const title = titleElement ? titleElement.textContent : document.title;

    // Extract stylesheet links
    const stylesheets = Array.from(
      doc.querySelectorAll('link[rel="stylesheet"]')
    ).map((link) => ({
      href: link.getAttribute("href"),
      media: link.getAttribute("media"),
    }));

    return {
      content: contentElement.innerHTML,
      title: title,
      stylesheets: stylesheets,
      doc: doc, // Keep full document for potential future use
    };
  },

  // Update the page with new content
  updatePage(extracted) {
    // Update content
    const contentContainer = document.querySelector(
      this.config.contentSelector
    );
    if (!contentContainer) {
      throw new Error("Could not find content container");
    }

    console.log("[Router] Replacing page content");

    // Update stylesheets
    this.updateStylesheets(extracted.stylesheets);

    // IMPORTANT: Preserve currently playing audio element
    let preservedAudio = null;
    const playerState = getPlayerState();

    if (playerState.audioElement && playerState.type === "recording") {
      console.log("[Router] Preserving currently playing audio element");
      preservedAudio = playerState.audioElement;
      // Remove from DOM temporarily (but keep reference)
      if (preservedAudio.parentNode) {
        preservedAudio.parentNode.removeChild(preservedAudio);
      }
    }

    contentContainer.innerHTML = extracted.content;

    // Re-attach preserved audio element to body (hidden)
    if (preservedAudio) {
      console.log("[Router] Re-attaching preserved audio element");
      document.body.appendChild(preservedAudio);
    }

    // Update title
    document.title = extracted.title;

    // Update meta tags if needed (could be extended)
    // For now, just update title
  },

  // Mark initial page stylesheets as dynamic so they can be managed by router
  markInitialStylesheetsAsDynamic() {
    const stylesheets = document.querySelectorAll(
      'link[rel="stylesheet"]:not([data-dynamic])'
    );
    stylesheets.forEach((link) => {
      // Mark all stylesheets except those that should always persist
      const href = link.getAttribute("href");
      // Always keep reset.css, common.css, and player.css
      if (
        !href.includes("reset.css") &&
        !href.includes("common.css") &&
        !href.includes("player.css")
      ) {
        console.log("[Router] Marking stylesheet as dynamic:", href);
        link.setAttribute("data-dynamic", "true");
      }
    });
  },

  // Update stylesheets dynamically
  updateStylesheets(newStylesheets) {
    console.log("[Router] Updating stylesheets");

    // Remove old dynamically-loaded stylesheets
    const oldDynamicLinks = document.querySelectorAll(
      'link[rel="stylesheet"][data-dynamic]'
    );
    oldDynamicLinks.forEach((link) => {
      console.log("[Router] Removing old stylesheet:", link.href);
      link.remove();
    });

    // Get currently loaded stylesheets (that aren't dynamic)
    const currentStylesheets = Array.from(
      document.querySelectorAll('link[rel="stylesheet"]:not([data-dynamic])')
    ).map((link) => {
      const href = link.getAttribute("href");
      return href;
    });

    // Add new stylesheets that aren't already loaded
    newStylesheets.forEach((stylesheet) => {
      let normalizedHref = stylesheet.href;

      // Check if already loaded (compare normalized paths)
      const alreadyLoaded = currentStylesheets.some((existing) => {
        // Handle relative paths - both '../file.css' and 'file.css' should match
        const existingNormalized = existing.replace(/^\.\.\//, "");
        const newNormalized = normalizedHref.replace(/^\.\.\//, "");
        return (
          existingNormalized === newNormalized || existing === normalizedHref
        );
      });

      if (!alreadyLoaded) {
        console.log("[Router] Adding new stylesheet:", normalizedHref);
        const link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = normalizedHref;
        link.setAttribute("data-dynamic", "true");
        if (stylesheet.media) {
          link.media = stylesheet.media;
        }
        document.head.appendChild(link);
      }
    });
  },

  // Handle browser back/forward navigation
  handlePopState(e) {
    console.log("[Router] Handling popstate event");

    if (e.state && e.state.path) {
      console.log("[Router] Navigating to:", e.state.path);
      this.navigate(e.state.path, true); // Skip pushState since we're already in history
    } else {
      // No state, might be initial page load or hash change
      const path = window.location.pathname;
      if (path !== this.currentPath) {
        console.log("[Router] Navigating to:", path);
        this.navigate(path, true);
      }
    }
  },
};

// Initialize when DOM is ready
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", () => Router.init());
} else {
  Router.init();
}

// Export for potential external use
export default Router;
