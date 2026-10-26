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
    if (menu && !menu.querySelector('a[href="/playground/"]')) {
        var item = document.createElement('li');
        var link = document.createElement('a');
        link.href = '/playground/';
        link.textContent = 'Playground';
        item.appendChild(link);
        menu.appendChild(item);
    }
});
