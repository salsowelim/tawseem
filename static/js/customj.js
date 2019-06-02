$(document).ready(function() {
  $('#0show').click(function() {
    $('#0demo').collapse('show');
  });

  $('#0hide').click(function() {
    $('#0demo').collapse('hide');
  });

  $('#show').click(function() {
    $('#demo').collapse('show');
  });

  $('#hide').click(function() {
    $('#demo').collapse('hide');
  });

  $('#hide2').click(function() {
    $('#demo').collapse('hide');
  });

  $.viewMap = {
    '0': $([]),
    'view1': $('#view1'),
    'view2': $('#view1, #view2'),
    'view3': $('#view1, #view2, #view3'),
    'view4': $('#view1, #view2, #view3, #view4')
  };
  $.each($.viewMap, function() {
    this.hide();
  });

  $('#viewSelector').change(function() {
    // hide all
    $.each($.viewMap, function() {
      this.hide();
    });
    // show current
    $.viewMap[$(this).val()].show();
  });

  $.viewMap2 = {
    '0': $([]),
    '2view1': $('#2view1'),
    '2view2': $('#2view1, #2view2'),
    '2view3': $('#2view1, #2view2, #2view3'),
    '2view4': $('#2view1, #2view2, #2view3, #2view4')
  };
  $.each($.viewMap2, function() {
    this.hide();
  });
  $('#viewSelector2').change(function() {
    // hide all
    $.each($.viewMap2, function() {
      this.hide();
    });
    // show current
    $.viewMap2[$(this).val()].show();
  });
 });