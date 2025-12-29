/**
 * Sidebar Navigation Toggle
 * Handles sidebar open/close for mobile and desktop
 */

(function() {
    'use strict';

    const sidebar = document.querySelector('[data-sidebar]');
    const sidebarToggle = document.querySelector('[data-sidebar-toggle]');
    const sidebarOverlay = document.querySelector('[data-sidebar-overlay]');

    if (!sidebar || !sidebarToggle || !sidebarOverlay) {
        return;
    }

    function openSidebar() {
        sidebar.classList.add('is-open');
        sidebarOverlay.classList.add('is-open');
        sidebarToggle.setAttribute('aria-expanded', 'true');
        document.body.style.overflow = 'hidden'; // Prevent scrolling when sidebar is open on mobile
    }

    function closeSidebar() {
        sidebar.classList.remove('is-open');
        sidebarOverlay.classList.remove('is-open');
        sidebarToggle.setAttribute('aria-expanded', 'false');
        document.body.style.overflow = '';
    }

    function toggleSidebar() {
        if (sidebar.classList.contains('is-open')) {
            closeSidebar();
        } else {
            openSidebar();
        }
    }

    // Toggle button click
    sidebarToggle.addEventListener('click', toggleSidebar);

    // Overlay click to close
    sidebarOverlay.addEventListener('click', closeSidebar);

    // Close sidebar on escape key
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape' && sidebar.classList.contains('is-open')) {
            closeSidebar();
        }
    });

    // Close sidebar when navigating on mobile
    const sidebarLinks = sidebar.querySelectorAll('.sidebar-link, .sidebar-logout-button');
    sidebarLinks.forEach(link => {
        link.addEventListener('click', function() {
            // Only close on mobile
            if (window.innerWidth < 1024) {
                closeSidebar();
            }
        });
    });

    // Auto-close sidebar on window resize from mobile to desktop
    let resizeTimer;
    window.addEventListener('resize', function() {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(function() {
            if (window.innerWidth >= 1024 && sidebar.classList.contains('is-open')) {
                closeSidebar();
            }
        }, 250);
    });

    // Set active link based on current path
    const currentPath = window.location.pathname;
    sidebarLinks.forEach(link => {
        if (link.classList.contains('sidebar-link')) {
            const linkPath = link.getAttribute('href');
            if (linkPath === currentPath || (currentPath === '/' && linkPath === '/')) {
                link.classList.add('is-active');
            }
        }
    });
})();
