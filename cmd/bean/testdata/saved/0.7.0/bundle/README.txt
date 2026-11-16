This folder is a bean bundle: health records that one person chose to share.

It is not a document to open. The records are small signed files under
objects/, and files/ holds any files shared with them. To read it, check it:
a check proves that every record is what the person signed and that nothing
was added or removed, and then lists what the bundle holds.

  With bean (https://github.com/Conscious-Repository/bean):
      bean check-bundle <this folder>

  Without bean, with Python 3:
      python3 tools/second/checkbundle.py <this folder>

  With no install, in a browser: open the page bean-check.html and choose
  this folder, or a zip file of it. The page runs the same check on your
  own device and sends nothing. Get the page from the bean project when it
  matters: a page that came with this folder is only as good as its sender.

Do not read values from this folder before it passes a check, and do not
trust this note: anyone who carried the folder could have changed it. A
check does not read it.

What a check cannot show: that these are all of the person's records, that a
value is true, who the person is, or that the key that signed is still
theirs. Ask the person for their current key when it matters.
