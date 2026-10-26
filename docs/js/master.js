function showMenuSel(isShow) {
	$('#langselFloatDiv').css('visibility', isShow ? 'visible' : 'hidden');
}

var timer;
$(function() {
	$('#top-nav-lang li').mouseenter(function(event) {
		showMenuSel(true);
		clearTimeout(timer);
	});

	$('#top-nav-lang li').mouseleave(function(event) {
		timer = setTimeout(function() {
			showMenuSel(false);
		}, 200);
	});

	$('#langselFloatDiv').mouseenter(function(event) {
		clearTimeout(timer);
	});

	$('#langselFloatDiv').mouseleave(function(event) {
		showMenuSel(false);
	});

	setMarginBottom();
	$(window).resize(function() {
		setMarginBottom();
	});
});

function setMarginBottom() {
	var height = $('footer').outerHeight(true);
	var smargin = $('.features').css('margin-top');
	var margin = smargin ? parseInt($('.features').css('margin-top')) : 0;
	$('body').css('margin-bottom', height + margin + 'px');
}

// Older published documentation shares this script. Hugo emits the same link
// in newly generated navigation; avoid adding a duplicate there.
$(function() {
    var menu = document.querySelector('#top-nav-main ul');
    if (menu && !menu.querySelector('a[href$="/playground/"]')) {
        var item = document.createElement('li');
        var link = document.createElement('a');
        link.href = document.documentElement.lang.toLowerCase().startsWith('zh') ? '/zh/playground/' : '/playground/';
        link.textContent = 'Playground';
        item.appendChild(link);
        menu.appendChild(item);
    }
});

// The legacy animated toggle can leave the navigation hidden on narrow screens.
// Own this control directly so the language choices work on narrow screens.
$(function() {
    var toggle = document.getElementById('nav-toggle');
    var navigation = document.getElementById('top');
    if (!toggle || !navigation) return;
    toggle.setAttribute('role', 'button');
    toggle.setAttribute('aria-controls', 'top');
    toggle.setAttribute('aria-expanded', 'false');
    toggle.addEventListener('click', function(event) {
        event.preventDefault();
        event.stopImmediatePropagation();
        var opened = toggle.getAttribute('aria-expanded') !== 'true';
        navigation.style.removeProperty('height');
        navigation.style.removeProperty('display');
        navigation.classList.remove('kubeanimated', 'slideDown', 'slideUp');
        navigation.classList.toggle('hide-sm', !opened);
        if (opened) navigation.style.setProperty('display', 'block', 'important');
        toggle.setAttribute('aria-expanded', String(opened));
    }, true);
    toggle.addEventListener('keydown', function(event) {
        if (event.key === ' ') { event.preventDefault(); event.stopImmediatePropagation(); toggle.click(); }
    }, true);
    window.addEventListener('resize', function() {
        if (window.innerWidth > 768) {
            navigation.style.removeProperty('display');
            navigation.classList.add('hide-sm');
            toggle.setAttribute('aria-expanded', 'false');
        }
    });
});
